package handler

import (
	"errors"
	"net/http"
	"strings"

	"wendaoiotpanel/internal/events"
	"wendaoiotpanel/internal/model"
	"wendaoiotpanel/internal/store"
	cryptopkg "wendaoiotpanel/pkg/crypto"

	"github.com/gin-gonic/gin"
)

const maxPreregisterItems = 500

// assertProductBelongsToTenant 产品归属校验：超管（tenantID=nil）直通。
func (h *Handler) assertProductBelongsToTenant(productKey string, tenantID *uint) (*model.Product, bool) {
	p, err := h.store.GetProductByKey(productKey)
	if err != nil {
		return nil, false
	}
	if tenantID != nil && p.TenantID != *tenantID {
		return nil, false
	}
	return p, true
}

// resolveTenantID 调用者目标租户：租户管理员强制本租户；超管用 body 指定值。
func resolveTenantID(role string, authTenant *uint, bodyTenant uint) (uint, bool) {
	if role == model.RoleTenantAdmin {
		if authTenant == nil {
			return 0, false
		}
		return *authTenant, true
	}
	return bodyTenant, bodyTenant != 0
}

// CreateProduct POST /products  body: {name, tenant_id(超管必填)}
// 产品密钥（ProductSecret）经 bcrypt 存储，明文仅本次响应返回一次。
func (h *Handler) CreateProduct(c *gin.Context) {
	role, authTenant := getAuthInfo(c)
	var req struct {
		Name     string `json:"name" binding:"required"`
		TenantID uint   `json:"tenant_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	tenantID, ok := resolveTenantID(role, authTenant, req.TenantID)
	if !ok {
		fail(c, http.StatusBadRequest, "请指定所属租户")
		return
	}

	// ProductKey 随机生成（无 -/_，避免与引导用户名分隔符组合产生歧义），冲突重试 3 次。
	var productKey string
	for attempt := 0; attempt < 3; attempt++ {
		k, err := cryptopkg.RandomPassword(16)
		if err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := h.store.GetProductByKey(k); err != nil {
			productKey = k
			break
		}
	}
	if productKey == "" {
		fail(c, http.StatusInternalServerError, "产品标识生成冲突，请重试")
		return
	}

	secret, err := cryptopkg.RandomPassword(20)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	p := &model.Product{
		ProductKey:    productKey,
		ProductSecret: cryptopkg.MustHashPassword(secret),
		TenantID:      tenantID,
		Name:          req.Name,
		DynRegEnabled: true,
	}
	if err := h.store.CreateProduct(p); err != nil {
		if strings.Contains(err.Error(), "1062") || strings.Contains(err.Error(), "Duplicate") {
			fail(c, http.StatusBadRequest, "产品标识冲突，请重试")
			return
		}
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{
		"product":        p,
		"product_secret": secret,
		"secret_note":    "产品密钥仅此一次返回，烧录到同型号设备固件，请妥善保存",
	})
}

// ListProducts GET /products  超管可 ?tenant_id= 过滤，租户管理员仅见本租户产品。
func (h *Handler) ListProducts(c *gin.Context) {
	role, authTenant := getAuthInfo(c)
	if role == model.RoleTenantAdmin {
		if authTenant == nil {
			fail(c, http.StatusBadRequest, "租户信息异常")
			return
		}
		products, err := h.store.ListProducts(authTenant)
		if err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		success(c, products)
		return
	}
	var q struct {
		TenantID *uint `form:"tenant_id"`
	}
	c.ShouldBindQuery(&q)
	products, err := h.store.ListProducts(q.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, products)
}

// UpdateProduct PUT /products/:key  body: {name?, dyn_reg_enabled?}
func (h *Handler) UpdateProduct(c *gin.Context) {
	key := c.Param("key")
	_, tenantID := getAuthInfo(c)
	p, ok := h.assertProductBelongsToTenant(key, tenantID)
	if !ok {
		fail(c, http.StatusForbidden, "产品不存在或无权操作")
		return
	}
	var req struct {
		Name          string `json:"name"`
		DynRegEnabled *bool  `json:"dyn_reg_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" && req.DynRegEnabled == nil {
		fail(c, http.StatusBadRequest, "无可更新字段")
		return
	}
	name := req.Name
	if name == "" {
		name = p.Name
	}
	if err := h.store.UpdateProduct(key, name, req.DynRegEnabled); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// ResetProductSecret POST /products/:key/secret/reset  旧产品密钥立即失效。
func (h *Handler) ResetProductSecret(c *gin.Context) {
	key := c.Param("key")
	_, tenantID := getAuthInfo(c)
	if _, ok := h.assertProductBelongsToTenant(key, tenantID); !ok {
		fail(c, http.StatusForbidden, "产品不存在或无权操作")
		return
	}
	secret, err := cryptopkg.RandomPassword(20)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SetProductSecret(key, cryptopkg.MustHashPassword(secret)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, gin.H{
		"product_key":    key,
		"product_secret": secret,
		"secret_note":    "新产品密钥仅此一次返回；待激活设备需改用新密钥引导注册",
	})
}

// DeleteProduct DELETE /products/:key  产品下仍有设备（含待激活）时拒绝。
func (h *Handler) DeleteProduct(c *gin.Context) {
	key := c.Param("key")
	_, tenantID := getAuthInfo(c)
	if _, ok := h.assertProductBelongsToTenant(key, tenantID); !ok {
		fail(c, http.StatusForbidden, "产品不存在或无权操作")
		return
	}
	if err := h.store.DeleteProductByKey(key); err != nil {
		if errors.Is(err, store.ErrProductInUse) {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// BatchPreregister POST /devices/batch-preregister
// body: {product_key, project_id, items:[{id, name?}]}（items ≤ 500）。
// 产品与项目必须同属调用者租户；逐行预录（密钥留空=待激活），逐行返回结果，不做全有全无事务。
func (h *Handler) BatchPreregister(c *gin.Context) {
	_, tenantID := getAuthInfo(c)
	var req struct {
		ProductKey string `json:"product_key"`
		ProjectID  uint   `json:"project_id"`
		Items      []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	req.ProductKey = strings.TrimSpace(req.ProductKey)
	if req.ProductKey == "" || req.ProjectID == 0 {
		fail(c, http.StatusBadRequest, "product_key 与 project_id 必填")
		return
	}
	if len(req.Items) == 0 {
		fail(c, http.StatusBadRequest, "items 不能为空")
		return
	}
	if len(req.Items) > maxPreregisterItems {
		fail(c, http.StatusBadRequest, "单次最多预录 500 个 SN")
		return
	}
	product, ok := h.assertProductBelongsToTenant(req.ProductKey, tenantID)
	if !ok {
		fail(c, http.StatusForbidden, "产品不存在或无权操作")
		return
	}
	proj, err := h.store.GetProjectByID(req.ProjectID)
	if err != nil || (tenantID != nil && proj.TenantID != *tenantID) {
		fail(c, http.StatusForbidden, "项目不存在或无权操作")
		return
	}
	if proj.TenantID != product.TenantID {
		fail(c, http.StatusBadRequest, "产品与项目不属于同一租户")
		return
	}

	type rowResult struct {
		ID  string `json:"id"`
		OK  bool   `json:"ok"`
		Msg string `json:"msg,omitempty"`
	}
	results := make([]rowResult, 0, len(req.Items))
	seen := make(map[string]bool, len(req.Items))
	succeeded, failed := 0, 0
	for _, it := range req.Items {
		sn := strings.TrimSpace(it.ID)
		r := rowResult{ID: sn}
		switch {
		case sn == "":
			r.Msg = "SN 为空"
		case !deviceIDPattern.MatchString(sn):
			r.Msg = "SN 不合法：仅允许 1-64 位字母/数字/下划线/短横线"
		case seen[sn]:
			r.Msg = "批次内 SN 重复"
		default:
			name := strings.TrimSpace(it.Name)
			if name == "" {
				name = sn
			}
			d := &model.Device{
				ID:        sn,
				ProjectID: req.ProjectID,
				TenantID:  proj.TenantID,
				Name:      name,
				Status:    model.DeviceStatusOffline,
				Enabled:   true,
				ProductID: product.ID,
			}
			if err := h.store.PreregisterDevice(d); err != nil {
				if strings.Contains(err.Error(), "1062") || strings.Contains(err.Error(), "Duplicate") {
					r.Msg = "SN 已存在"
				} else {
					r.Msg = err.Error()
				}
			} else {
				r.OK = true
			}
			seen[sn] = true
		}
		if r.OK {
			succeeded++
		} else {
			failed++
		}
		results = append(results, r)
	}
	success(c, gin.H{
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	})
}

// ReactivateDevice POST /devices/:deviceId/reactivate
// 清空已签发的一机一密，设备回到待激活状态（旧密钥立即失效），可重新引导注册。
// 仅限一型一密设备（product_id>0）；若设备当前在线，发送通知后踢线。
func (h *Handler) ReactivateDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	_, tenantID := getAuthInfo(c)
	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		fail(c, http.StatusNotFound, "设备不存在")
		return
	}
	if tenantID != nil {
		p, perr := h.store.GetProjectByID(dev.ProjectID)
		if perr != nil || p.TenantID != *tenantID {
			forbidden(c, "无权操作此设备")
			return
		}
	}
	if dev.ProductID == 0 {
		fail(c, http.StatusBadRequest, "该设备非一型一密接入，不支持重新激活")
		return
	}
	if err := h.store.ClearDeviceActivation(deviceID); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 在线设备：旧密钥已失效，主动通知并踢线，促使其重新走引导注册。
	notifyAndKick(deviceID, "", "reactivated", "设备已被管理员重置为待激活，本连接被平台断开", true)
	if h.bus != nil && dev.TenantID != 0 {
		h.bus.BroadcastToTenant(dev.TenantID, events.Message{
			Type: "device_status",
			Data: map[string]interface{}{"device_id": deviceID, "online": false},
		})
	}
	success(c, gin.H{"id": deviceID, "activated": false})
}
