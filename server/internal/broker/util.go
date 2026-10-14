package broker

import (
	"encoding/json"
	"time"

	"wendaoiotpanel/internal/protocol"
)

// marshalKickedNotice 序列化被踢通知（协议结构见 protocol.KickedNotice）。
func marshalKickedNotice(reason, msg string) ([]byte, error) {
	return json.Marshal(protocol.KickedNotice{Reason: reason, Msg: msg, Ts: time.Now().UnixMilli()})
}
