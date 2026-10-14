package store

import (
	"wendaoiotpanel/internal/model"

	"gorm.io/gorm"
)

// TrafficRange 仪表盘历史流量聚合预设：桶大小（秒）与返回点数。
type TrafficRange struct {
	BucketSec int64
	Points    int
}

// TrafficRanges 可选历史窗口（点数控制在 60–168，兼顾平滑度与查询开销）：
//
//	1h ：60 点 × 60s（1 分钟桶）
//	24h：144 点 × 600s（10 分钟桶）
//	7d ：168 点 × 3600s（1 小时桶）
var TrafficRanges = map[string]TrafficRange{
	"1h":  {BucketSec: 60, Points: 60},
	"24h": {BucketSec: 600, Points: 144},
	"7d":  {BucketSec: 3600, Points: 168},
}

type trafficBucket struct {
	Bucket int64 `gorm:"column:bucket"`
	Cnt    int64 `gorm:"column:cnt"`
}

// MessageTraffic 按固定时间桶统计设备上行（device_data，ts 毫秒）与平台下行
// （control_logs，created_at）消息条数。
//
// startSec 为第一桶起点（Unix 秒，已对齐到桶边界），返回区间 [startSec, startSec+points*bucketSec)，
// in/out 长度恒为 points、按时间正序，无数据补 0；末桶为“当前进行中”的部分桶。
// tenantID=nil 聚合全部租户（超管全局视图），否则仅统计该租户项目下设备。
func (s *Store) MessageTraffic(tenantID *uint, startSec int64, bucketSec int64, points int) ([]int64, []int64, error) {
	in := make([]int64, points)
	out := make([]int64, points)
	startBucket := startSec / bucketSec
	endSec := startSec + int64(points)*bucketSec

	// 租户作用域：tenant → projectIDs → deviceIDs（与 GetDashboardStats 口径一致）
	scoped := func(q *gorm.DB) *gorm.DB { return q }
	if tenantID != nil {
		var projectIDs []uint
		s.db.Model(&model.Project{}).Where("tenant_id = ?", *tenantID).Pluck("id", &projectIDs)
		if len(projectIDs) == 0 {
			return in, out, nil
		}
		var deviceIDs []string
		s.db.Model(&model.Device{}).Where("project_id IN ?", projectIDs).Pluck("id", &deviceIDs)
		if len(deviceIDs) == 0 {
			return in, out, nil
		}
		scoped = func(q *gorm.DB) *gorm.DB { return q.Where("device_id IN ?", deviceIDs) }
	}

	fill := func(rows []trafficBucket, dst []int64) {
		for _, r := range rows {
			idx := r.Bucket - startBucket
			if idx >= 0 && idx < int64(points) {
				dst[idx] = r.Cnt
			}
		}
	}

	// 上行：ts 为 Unix 毫秒，桶号换算到秒再除桶大小
	var inRows []trafficBucket
	qIn := scoped(s.db.Model(&model.DeviceData{}).
		Select("FLOOR(ts / 1000 / ?) AS bucket, COUNT(*) AS cnt", bucketSec).
		Where("ts >= ? AND ts < ?", startSec*1000, endSec*1000).
		Group("bucket"))
	if err := qIn.Scan(&inRows).Error; err != nil {
		return nil, nil, err
	}
	fill(inRows, in)

	// 下行：created_at 为日期时间，用 UNIX_TIMESTAMP 取秒分桶
	var outRows []trafficBucket
	qOut := scoped(s.db.Model(&model.ControlLog{}).
		Select("FLOOR(UNIX_TIMESTAMP(created_at) / ?) AS bucket, COUNT(*) AS cnt", bucketSec).
		Where("created_at >= FROM_UNIXTIME(?) AND created_at < FROM_UNIXTIME(?)", startSec, endSec).
		Group("bucket"))
	if err := qOut.Scan(&outRows).Error; err != nil {
		return nil, nil, err
	}
	fill(outRows, out)

	return in, out, nil
}
