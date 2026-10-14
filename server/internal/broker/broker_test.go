package broker

import (
	"errors"
	"strings"
	"testing"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	cryptopkg "wendaoiotpanel/pkg/crypto"

	"wendaoiotpanel/internal/model"
	"wendaoiotpanel/internal/protocol"
)

// errNotFound 模拟 gorm.ErrRecordNotFound 语义（测试只关心 error != nil）。
var errNotFound = errors.New("record not found")

// ---- 测试用假 store ----

type fakeStore struct {
	devices map[string]*model.Device // deviceID → auth record
	byKey   map[string]*model.Product
	status  map[string]bool         // 在线状态记录（deviceID → online）
	sink    []fakeStatusEvent       // 上报给 sink 的上下线事件
}

type fakeStatusEvent struct {
	deviceID string
	online   bool
	clientID string
	tenantID uint
}

type fakeSink struct{ f *fakeStore }

func (s fakeSink) SetDeviceRuntime(deviceID string, online bool, clientID string, tenantID uint) {
	s.f.status[deviceID] = online
	s.f.sink = append(s.f.sink, fakeStatusEvent{deviceID, online, clientID, tenantID})
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		devices: map[string]*model.Device{},
		byKey:   map[string]*model.Product{},
		status:  map[string]bool{},
	}
}

func (f *fakeStore) GetDeviceAuthRecord(id string) (*model.Device, error) {
	if d, ok := f.devices[id]; ok {
		return d, nil
	}
	return nil, errNotFound
}

func (f *fakeStore) GetProductByKey(key string) (*model.Product, error) {
	if p, ok := f.byKey[key]; ok {
		return p, nil
	}
	return nil, errNotFound
}

func (f *fakeStore) GetDevice(id string) (*model.Device, error) {
	return f.GetDeviceAuthRecord(id)
}

// ---- 构造辅助 ----

func connectPacket(username, password, clientID string) packets.Packet {
	return packets.Packet{
		Connect: packets.ConnectParams{
			Username:         []byte(username),
			Password:         []byte(password),
			ClientIdentifier: clientID,
		},
	}
}

func newTestBroker(t *testing.T) (*Broker, *fakeStore) {
	t.Helper()
	s := newFakeStore()
	b, err := New(Config{
		ListenPort:     0, // 单测不监听
		ServerUsername: "wendao_server",
		ServerPassword: "srv-pass-123",
	}, s, fakeSink{f: s})
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	return b, s
}

func seedDevice(t *testing.T, s *fakeStore, id, secret string, enabled bool) {
	t.Helper()
	hashed, err := cryptopkg.HashPassword(secret)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	s.devices[id] = &model.Device{ID: id, DeviceSecret: hashed, Enabled: enabled, TenantID: 1}
}

func newClient(username string) *mqtt.Client {
	return &mqtt.Client{
		ID:         username + "-cid",
		Properties: mqtt.ClientProperties{Username: []byte(username)},
	}
}

// ---- 认证矩阵 ----

func TestAuthServerAccount(t *testing.T) {
	b, _ := newTestBroker(t)
	h := &WendaoHook{broker: b}

	if !h.OnConnectAuthenticate(newClient("x"), connectPacket("wendao_server", "srv-pass-123", "server")) {
		t.Fatal("server account with correct password should allow")
	}
	if h.OnConnectAuthenticate(newClient("x"), connectPacket("wendao_server", "wrong", "server")) {
		t.Fatal("server account with wrong password should deny")
	}
}

func TestAuthDeviceMatrix(t *testing.T) {
	b, s := newTestBroker(t)
	h := &WendaoHook{broker: b}
	seedDevice(t, s, "dev001", "dev-secret-001", true)

	cases := []struct {
		name string
		user string
		pass string
		want bool
	}{
		{"ok", "dev001", "dev-secret-001", true},
		{"wrong password", "dev001", "nope", false},
		{"unknown device", "ghost", "x", false},
		{"anonymous", "", "x", false},
	}
	for _, c := range cases {
		got := h.OnConnectAuthenticate(newClient(c.user), connectPacket(c.user, c.pass, "cid-"+c.user))
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}

	// 禁用设备
	s.devices["dev001"].Enabled = false
	if h.OnConnectAuthenticate(newClient("dev001"), connectPacket("dev001", "dev-secret-001", "cid")) {
		t.Error("disabled device should deny")
	}

	// 未下发密钥（待激活）
	s.devices["dev001"].Enabled = true
	s.devices["dev001"].DeviceSecret = ""
	if h.OnConnectAuthenticate(newClient("dev001"), connectPacket("dev001", "dev-secret-001", "cid")) {
		t.Error("device without secret should deny")
	}
}

func TestAuthBootstrapMatrix(t *testing.T) {
	b, s := newTestBroker(t)
	h := &WendaoHook{broker: b}

	prodSecret, _ := cryptopkg.HashPassword("prod-secret-xyz")
	// ProductKeyPattern 要求 [A-Za-z0-9]{8,40}，测试 key 须至少 8 位
	s.byKey["pk12345678"] = &model.Product{ID: 7, ProductKey: "pk12345678", ProductSecret: prodSecret, DynRegEnabled: true}
	s.devices["SN001"] = &model.Device{ID: "SN001", ProductID: 7, Enabled: true} // 未激活

	user := "SN001" + protocol.BootstrapSep + "pk12345678"

	cases := []struct {
		name string
		user string
		pass string
		want bool
	}{
		{"ok", user, "prod-secret-xyz", true},
		{"bad product secret", user, "wrong", false},
		{"unknown product", "SN001&nopk", "prod-secret-xyz", false},
		{"malformed product key", "SN999&pk123", "prod-secret-xyz", false}, // key 不足 8 位，走普通设备分支被拒
		{"sn not preregistered", "SN999&pk12345678", "prod-secret-xyz", false},
		{"no sep is normal device path", "SN001", "prod-secret-xyz", false},
	}
	for _, c := range cases {
		got := h.OnConnectAuthenticate(newClient(c.user), connectPacket(c.user, c.pass, "cid"))
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}

	// 已激活（有一机一密）→ 引导拒绝
	s.devices["SN001"].DeviceSecret = "activated-secret"
	if h.OnConnectAuthenticate(newClient(user), connectPacket(user, "prod-secret-xyz", "cid")) {
		t.Error("already activated device bootstrap should deny")
	}
}

// bootstrapSep 必须与 protocol.BootstrapSep 保持一致。
func TestBootstrapSepConsistency(t *testing.T) {
	if bootstrapSep != protocol.BootstrapSep {
		t.Fatalf("bootstrapSep %q != protocol.BootstrapSep %q", bootstrapSep, protocol.BootstrapSep)
	}
}

// ---- ACL 矩阵 ----

func TestACLServerAccountAllAllow(t *testing.T) {
	b, _ := newTestBroker(t)
	h := &WendaoHook{broker: b}
	cl := newClient("wendao_server")
	if !h.OnACLCheck(cl, "wendao/any/kicked", true) {
		t.Error("server publish should allow")
	}
	if !h.OnACLCheck(cl, "wendao/+/data", false) {
		t.Error("server subscribe wildcard should allow")
	}
}

func TestACLDeviceMatrix(t *testing.T) {
	b, _ := newTestBroker(t)
	h := &WendaoHook{broker: b}
	cl := newClient("dev001")

	pubOK := []string{
		"wendao/dev001/data",
		"wendao/dev001/control/ack",
		"wendao/dev001/ping/ack",
		"wendao/dev001/ota/ack",
		"wendao/dev001/ota/progress",
		"wendao/dev001/status",
		"wendao/dev001/peer",
	}
	pubDeny := []string{
		"wendao/dev002/data",                 // 他人前缀
		"wendao/dev001/control",              // 白名单外后缀（下行专属）
		"wendao/dev001/ping",                 // 白名单外
		"wendao/dev001/kicked",               // 平台专属
		"wendao/dev001/data/extra",           // 后缀不精确匹配
		"other/dev001/data",                  // 首段错
		"wendao/register/SN001/req",          // 注册主题非设备可发
		"$SYS/brokers/x/clients/y/connected", // $SYS
	}
	for _, tp := range pubOK {
		if !h.OnACLCheck(cl, tp, true) {
			t.Errorf("publish %q should allow", tp)
		}
	}
	for _, tp := range pubDeny {
		if h.OnACLCheck(cl, tp, true) {
			t.Errorf("publish %q should deny", tp)
		}
	}

	// 订阅：本人前缀全放行（含通配），他人/占位符/$SYS 拒绝
	subOK := []string{
		"wendao/dev001/data/ack",
		"wendao/dev001/control",
		"wendao/dev001/#",
		"wendao/dev001/+/ack",
	}
	subDeny := []string{
		"wendao/dev002/#",
		"wendao/+/data",
		"$SYS/#",
		"other/x",
	}
	for _, tp := range subOK {
		if !h.OnACLCheck(cl, tp, false) {
			t.Errorf("subscribe %q should allow", tp)
		}
	}
	for _, tp := range subDeny {
		if h.OnACLCheck(cl, tp, false) {
			t.Errorf("subscribe %q should deny", tp)
		}
	}

	// 匿名（无 username）一律拒绝
	if h.OnACLCheck(&mqtt.Client{ID: "anon"}, "wendao/dev001/data", true) {
		t.Error("anonymous ACL should deny")
	}
}

func TestACLBootstrapMatrix(t *testing.T) {
	b, _ := newTestBroker(t)
	h := &WendaoHook{broker: b}
	cl := newClient("SN001&pk12345678")

	if !h.OnACLCheck(cl, "wendao/register/SN001/req", true) {
		t.Error("bootstrap publish own register req should allow")
	}
	if !h.OnACLCheck(cl, "wendao/register/SN001/reply", false) {
		t.Error("bootstrap subscribe own register reply should allow")
	}
	if h.OnACLCheck(cl, "wendao/register/SN002/req", true) {
		t.Error("bootstrap publish other sn should deny")
	}
	if h.OnACLCheck(cl, "wendao/register/SN001/reply", true) {
		t.Error("bootstrap publish reply should deny")
	}
	if h.OnACLCheck(cl, "wendao/register/SN001/+", false) {
		t.Error("bootstrap subscribe wildcard should deny")
	}
}

// ---- sessionIndex ----

func TestSessionIndexAddRemoveList(t *testing.T) {
	s := newSessionIndex()
	a := newClient("dev1")
	a.ID = "cid-a"
	bb := newClient("dev1")
	bb.ID = "cid-b"

	s.Add("dev1", "cid-a", a)
	s.Add("dev1", "cid-b", bb)
	if got := len(s.List("dev1")); got != 2 {
		t.Fatalf("expect 2 sessions, got %d", got)
	}
	s.Remove("dev1", "cid-a")
	if got := len(s.List("dev1")); got != 1 {
		t.Fatalf("expect 1 session after remove, got %d", got)
	}
	s.Remove("dev1", "cid-b")
	if got := len(s.List("dev1")); got != 0 {
		t.Fatalf("expect 0 sessions, got %d", got)
	}
	// 空 username/clientID 防御
	s.Add("", "", a)
	s.Remove("x", "")
}

// ---- 上下线联动 ----

func TestSessionEstablishedOfflineLifecycle(t *testing.T) {
	b, s := newTestBroker(t)
	h := &WendaoHook{broker: b}
	seedDevice(t, s, "dev001", "x", true)

	cl := newClient("dev001")
	pk := connectPacket("dev001", "x", cl.ID)

	h.OnSessionEstablished(cl, pk)
	if !s.status["dev001"] {
		t.Error("device should be online after session established")
	}
	if len(b.sessions.List("dev001")) != 1 {
		t.Error("session should be indexed")
	}

	h.OnDisconnect(cl, nil, false)
	if s.status["dev001"] {
		t.Error("device should be offline after disconnect")
	}
	if len(b.sessions.List("dev001")) != 0 {
		t.Error("session should be removed from index")
	}
}

func TestSessionEstablishIgnoresServerAndBootstrap(t *testing.T) {
	b, s := newTestBroker(t)
	h := &WendaoHook{broker: b}

	h.OnSessionEstablished(newClient("wendao_server"), connectPacket("wendao_server", "p", "server"))
	h.OnSessionEstablished(newClient("SN1&pk"), connectPacket("SN1&pk", "p", "b1"))
	if len(s.status) != 0 {
		t.Errorf("server/bootstrap sessions must not touch device status, got %v", s.status)
	}
}

// ---- 互踢 ----

func TestKickByUsername(t *testing.T) {
	b, s := newTestBroker(t)
	h := &WendaoHook{broker: b}
	seedDevice(t, s, "dev001", "x", true)

	old := newClient("dev001")
	old.ID = "cid-old"
	h.OnSessionEstablished(old, connectPacket("dev001", "x", "cid-old"))

	// 踢线流程不阻塞、不 panic（未 attach 的 client DisconnectClient 报错但被吞掉，
	// 端到端互踢断言由部署后的 _verify_kicked2.py 覆盖；此处验证接口行为）。
	kicked, err := b.KickByUsername("dev001", "cid-new")
	if err != nil {
		t.Fatalf("kick: %v", err)
	}
	// excludeClientID 命中时绝不返回被踢列表
	kicked, err = b.KickByUsername("dev001", "cid-old")
	if err != nil {
		t.Fatalf("kick: %v", err)
	}
	if len(kicked) != 0 {
		t.Fatalf("expect no kick when excluded, got %v", kicked)
	}

	// 空 username 防御
	if k, _ := b.KickByUsername("", ""); k != nil {
		t.Fatal("empty username should no-op")
	}
}

func TestHookUsernameExtraction(t *testing.T) {
	if got := hookClientUsername(newClient("dev9")); got != "dev9" {
		t.Errorf("username extract: got %q", got)
	}
	if got := hookClientUsername(nil); got != "" {
		t.Errorf("nil client should empty, got %q", got)
	}
	if !isBootstrapUsername("SN1&pk") || isBootstrapUsername("plain") {
		t.Error("bootstrap username detection wrong")
	}
	if !strings.Contains("a&b", bootstrapSep) {
		t.Error("sanity")
	}
}
