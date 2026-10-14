package metrics

// 本包维护单服务节点的消息收发实时计数（内存环形桶），用于仪表盘消息流入/
// 流出速率（Broker 仪表盘口径）：每个租户每秒一个桶，保留最近 ringSize 秒；
// 累计值为进程启动以来计数，重启清零。多副本部署时各节点独立计数，仅作实时
// 近似值，业务历史量以数据库计数为准。

import (
	"sync"
	"time"
)

const ringSize = 60

// dir 消息方向：in=设备上报流入平台，out=平台下行流出设备。
type dir int

const (
	dirIn dir = iota
	dirOut
)

type bucket struct {
	sec int64 // Unix 秒
	in  int64
	out int64
}

// tenantRing 单租户环形缓冲。sec=0 的桶视为空（冷启动）。
type tenantRing struct {
	buckets [ringSize]bucket
}

var (
	mu     sync.Mutex
	rings  = map[uint]*tenantRing{}
	totals = map[uint][2]int64{} // tenantID -> [in, out] 进程累计
)

// AddIn 记录一条设备上行消息（归属租户，tenantID=0 表示尚未回填租户的消息）。
func AddIn(tenantID uint) { add(tenantID, dirIn) }

// AddOut 记录一条平台下行消息。
func AddOut(tenantID uint) { add(tenantID, dirOut) }

func add(tenantID uint, d dir) {
	now := time.Now().Unix()
	mu.Lock()
	defer mu.Unlock()
	r := rings[tenantID]
	if r == nil {
		r = &tenantRing{}
		rings[tenantID] = r
	}
	b := &r.buckets[now%ringSize]
	if b.sec != now {
		*b = bucket{sec: now}
	}
	if d == dirIn {
		b.in++
	} else {
		b.out++
	}
	t := totals[tenantID]
	if d == dirIn {
		t[0]++
	} else {
		t[1]++
	}
	totals[tenantID] = t
}

// Snapshot 实时速率快照。
//
//	tenantID>0 仅该租户；tenantID=nil 聚合全部租户（超管全局视图）。
//	InRate/OutRate 为最近 1 秒条数（取不到当前秒桶时为 0）；
//	SeriesIn/SeriesOut 为按时间正序、长度 points 的每秒计数（冷启动不足补 0）。
type Snapshot struct {
	InRate   int64   `json:"in_rate"`
	OutRate  int64   `json:"out_rate"`
	InTotal  int64   `json:"in_total"`
	OutTotal int64   `json:"out_total"`
	SeriesIn []int64 `json:"series_in"`
	SeriesOut []int64 `json:"series_out"`
}

// SnapshotPoints 返回用于仪表盘折线的最近 N 秒序列（N≤ringSize）。
func SnapshotPoints(tenantID *uint, points int) Snapshot {
	if points <= 0 || points > ringSize {
		points = 30
	}
	now := time.Now().Unix()
	mu.Lock()
	defer mu.Unlock()

	var snap Snapshot
	snap.SeriesIn = make([]int64, points)
	snap.SeriesOut = make([]int64, points)

	pick := func(r *tenantRing) {
		// i=0 是最旧点（now-points+1 秒），末尾为当前秒
		for i := 0; i < points; i++ {
			sec := now - int64(points-1-i)
			b := r.buckets[sec%ringSize]
			if b.sec == sec {
				snap.SeriesIn[i] += b.in
				snap.SeriesOut[i] += b.out
			}
		}
	}

	if tenantID != nil {
		if r := rings[*tenantID]; r != nil {
			pick(r)
		}
		if t, ok := totals[*tenantID]; ok {
			snap.InTotal, snap.OutTotal = t[0], t[1]
		}
	} else {
		for _, r := range rings {
			pick(r)
		}
		for _, t := range totals {
			snap.InTotal += t[0]
			snap.OutTotal += t[1]
		}
	}

	snap.InRate = snap.SeriesIn[points-1]
	snap.OutRate = snap.SeriesOut[points-1]
	return snap
}
