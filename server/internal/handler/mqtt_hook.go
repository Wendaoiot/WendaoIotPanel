package handler

import (
	"log"
	"time"
)

// SessionKicker 互踢能力（由独立 broker 经内部 HTTP 实现，main 注入；nil 禁用）。
// excludeClientID 用于新登录时排除新连接自身，避免异步踢线误踢自己。
type SessionKicker interface {
	KickDeviceSessions(deviceID, excludeClientID string) ([]string, error)
}

// KickedNotifier 被踢通知能力：主动断开前向设备发布原因（由 mqtt.Client 实现）。
type KickedNotifier interface {
	NotifyKicked(deviceID, reason, msg string)
}

// SetSessionKicker 注入互踢实现（main 启动时调用；nil 表示禁用）。
var sessionKicker SessionKicker

func SetSessionKicker(k SessionKicker) {
	sessionKicker = k
}

// SetKickedNotifier 注入被踢通知实现（main 启动时调用；nil 表示禁用）。
var kickedNotifier KickedNotifier

func SetKickedNotifier(n KickedNotifier) {
	kickedNotifier = n
}

// kickNoticeGrace 通知已入队到执行踢线之间预留的投递宽限：
// 让 broker 有时间把 QoS1 kicked 消息下发给被踢的在线会话（clean session=true 时
// 消息不持久化，此宽限是其收到原因的唯一窗口；clean session=false 重连后仍可补收）。
var kickNoticeGrace = 600 * time.Millisecond

// notifyAndKick 向设备发布断开原因（sendNotice=true）并执行踢线。
// excludeClientID 用于新登录异步踢线时排除新连接自身，传空踢全部。
// sendNotice=false 时只踢线（原因已在调用前另行发送，避免重复通知）。
// 通知失败只记日志，不影响踢出流程。
func notifyAndKick(deviceID, excludeClientID, reason, msg string, sendNotice bool) {
	if sendNotice && kickedNotifier != nil {
		kickedNotifier.NotifyKicked(deviceID, reason, msg)
	}
	if kickNoticeGrace > 0 {
		time.Sleep(kickNoticeGrace)
	}
	if sessionKicker != nil {
		kicked, err := sessionKicker.KickDeviceSessions(deviceID, excludeClientID)
		if err != nil {
			log.Printf("kick sessions for %s failed: %v", deviceID, err)
		} else if len(kicked) > 0 {
			log.Printf("kicked %d session(s) of %s: %v", len(kicked), deviceID, kicked)
		}
	}
}
