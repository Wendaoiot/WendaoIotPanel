package handler

import (
	"net/http"
	"time"
	"wendaoiotpanel/internal/store"

	"github.com/gin-gonic/gin"
)

// GetDashboardTraffic 返回历史流量分桶序列（消息条数），供仪表盘
// “近 1 小时 / 近 24 小时 / 近 7 天”折线使用。实时档仍走 /dashboard/stats 的内存环。
//
// GET /api/v1/dashboard/traffic?range=1h|24h|7d
func (h *Handler) GetDashboardTraffic(c *gin.Context) {
	_, tenantID := getAuthInfo(c)

	rngName := c.DefaultQuery("range", "1h")
	rng, ok := store.TrafficRanges[rngName]
	if !ok {
		fail(c, http.StatusBadRequest, "invalid range, want one of: 1h, 24h, 7d")
		return
	}

	// 末桶取“当前进行中”的桶，起点对齐到桶边界
	now := time.Now().Unix()
	curBucket := now / rng.BucketSec
	startSec := (curBucket - int64(rng.Points-1)) * rng.BucketSec

	in, out, err := h.store.MessageTraffic(tenantID, startSec, rng.BucketSec, rng.Points)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	success(c, gin.H{
		"range":      rngName,
		"bucket_sec": rng.BucketSec,
		"start_sec":  startSec,
		"end_sec":    startSec + int64(rng.Points)*rng.BucketSec,
		"points":     rng.Points,
		"series_in":  in,
		"series_out": out,
	})
}
