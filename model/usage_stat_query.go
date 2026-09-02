package model

import (
	"reflect"

	"gorm.io/gorm"
)

// 使用分析看板的聚合表查询。全部打主库(不碰 LOG_DB),日期为 'YYYY-MM-DD' 字符串闭区间。
// userIds 语义: nil = 不限制(管理员); 非 nil = 按 user_id IN 过滤(部门负责人范围),
// 空非 nil 切片 = 无可见用户,直接返回空结果而不是全量。

const usageStatUserIdChunkSize = 500 // SQLite/PG 参数上限保护

// queryUsageStatsInChunks 对 userIds 分块执行同一查询并合并结果。
// 注意 GORM 的 Scan 每次调用会重置目标切片(不追加),所以每块扫进临时切片再合并。
// 同一 user_id 只落在一个分块里,GROUP BY 结果不会跨块重复。
func queryUsageStatsInChunks(userIds []int, dest any, build func(tx *gorm.DB) *gorm.DB) error {
	if userIds == nil {
		return build(DB).Scan(dest).Error
	}
	if len(userIds) == 0 {
		return nil
	}
	destSlice := reflect.ValueOf(dest).Elem() // *[]T
	for start := 0; start < len(userIds); start += usageStatUserIdChunkSize {
		end := start + usageStatUserIdChunkSize
		if end > len(userIds) {
			end = len(userIds)
		}
		chunkDest := reflect.New(destSlice.Type()) // *[]T
		if err := build(DB.Where("user_id IN ?", userIds[start:end])).Scan(chunkDest.Interface()).Error; err != nil {
			return err
		}
		destSlice.Set(reflect.AppendSlice(destSlice, chunkDest.Elem()))
	}
	return nil
}

// UsageDailyUserRow 按 日期x用户 聚合,活跃趋势与 DAU/WAU/MAU 的数据源。
type UsageDailyUserRow struct {
	Date         string `json:"date"`
	UserId       int    `json:"user_id"`
	RequestCount int64  `json:"request_count"`
	FailCount    int64  `json:"fail_count"`
	Quota        int64  `json:"quota"`
	RefundQuota  int64  `json:"refund_quota"`
}

func QueryUsageDailyByDate(startDate, endDate string, userIds []int) ([]UsageDailyUserRow, error) {
	rows := []UsageDailyUserRow{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatDaily{}).
			Select("date, user_id, SUM(request_count) AS request_count, SUM(fail_count) AS fail_count, "+
				"SUM(quota) AS quota, SUM(refund_quota) AS refund_quota").
			Where("date >= ? AND date <= ?", startDate, endDate).
			Group("date, user_id")
	})
	return rows, err
}

// UsageTotals 区间总 KPI。净消耗 = Quota - RefundQuota,由调用方计算。
type UsageTotals struct {
	RequestCount     int64 `json:"request_count"`
	FailCount        int64 `json:"fail_count"`
	Quota            int64 `json:"quota"`
	RefundQuota      int64 `json:"refund_quota"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	ActiveUsers      int64 `json:"active_users"`
}

func QueryUsageDailyTotals(startDate, endDate string, userIds []int) (UsageTotals, error) {
	rows := []UsageTotals{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatDaily{}).
			Select("COALESCE(SUM(request_count),0) AS request_count, COALESCE(SUM(fail_count),0) AS fail_count, "+
				"COALESCE(SUM(quota),0) AS quota, COALESCE(SUM(refund_quota),0) AS refund_quota, "+
				"COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, "+
				"COUNT(DISTINCT CASE WHEN request_count > 0 THEN user_id END) AS active_users").
			Where("date >= ? AND date <= ?", startDate, endDate)
	})
	// 分块时 ActiveUsers 需要去重,不能跨块相加;改为单独统计。
	if userIds == nil || len(userIds) <= usageStatUserIdChunkSize {
		if len(rows) == 0 {
			return UsageTotals{}, err
		}
		return rows[0], err
	}
	if err != nil {
		return UsageTotals{}, err
	}
	merged := UsageTotals{}
	for _, r := range rows {
		merged.RequestCount += r.RequestCount
		merged.FailCount += r.FailCount
		merged.Quota += r.Quota
		merged.RefundQuota += r.RefundQuota
		merged.PromptTokens += r.PromptTokens
		merged.CompletionTokens += r.CompletionTokens
	}
	// 活跃用户跨块去重: 从日x用户行重算。
	activeSet := make(map[int]bool)
	dailyRows, err := QueryUsageDailyByDate(startDate, endDate, userIds)
	if err != nil {
		return UsageTotals{}, err
	}
	for _, r := range dailyRows {
		if r.RequestCount > 0 {
			activeSet[r.UserId] = true
		}
	}
	merged.ActiveUsers = int64(len(activeSet))
	return merged, nil
}

// UsageUserRow 按用户聚合(Top N 卡片)。
type UsageUserRow struct {
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	RequestCount int64  `json:"request_count"`
	FailCount    int64  `json:"fail_count"`
	Quota        int64  `json:"quota"`
	RefundQuota  int64  `json:"refund_quota"`
}

func QueryUsageByUser(startDate, endDate string, userIds []int) ([]UsageUserRow, error) {
	rows := []UsageUserRow{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatDaily{}).
			Select("user_id, MAX(username) AS username, SUM(request_count) AS request_count, "+
				"SUM(fail_count) AS fail_count, SUM(quota) AS quota, SUM(refund_quota) AS refund_quota").
			Where("date >= ? AND date <= ?", startDate, endDate).
			Group("user_id")
	})
	return rows, err
}

// UsageModelRow 按模型聚合(模型偏好卡片)。
type UsageModelRow struct {
	ModelName        string `json:"model_name"`
	RequestCount     int64  `json:"request_count"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

func QueryUsageByModel(startDate, endDate string, userIds []int) ([]UsageModelRow, error) {
	rows := []UsageModelRow{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatDaily{}).
			Select("model_name, SUM(request_count) AS request_count, SUM(quota) AS quota, "+
				"SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens").
			Where("date >= ? AND date <= ?", startDate, endDate).
			Group("model_name")
	})
	return rows, err
}

// UsageHourlyRow 按 日期x小时 聚合(7x24 热力图;星期几由 Go 从 date 推导)。
type UsageHourlyRow struct {
	Date         string `json:"date"`
	Hour         int    `json:"hour"`
	RequestCount int64  `json:"request_count"`
	Quota        int64  `json:"quota"`
}

func QueryUsageHourly(startDate, endDate string, userIds []int) ([]UsageHourlyRow, error) {
	rows := []UsageHourlyRow{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatHourly{}).
			Select("date, hour, SUM(request_count) AS request_count, SUM(quota) AS quota").
			Where("date >= ? AND date <= ?", startDate, endDate).
			Group("date, hour")
	})
	return rows, err
}

// UsageUserTableRow 按用户聚合的全量明细行(用户分析表格)。
// 活跃口径与看板一致: request_count 只计 consume 日志,active_days/last_active_date
// 都以 request_count > 0 为准。范围内零请求的用户不会出现在结果里(由调用方补)。
type UsageUserTableRow struct {
	UserId           int    `json:"user_id"`
	Username         string `json:"username"`
	RequestCount     int64  `json:"request_count"`
	FailCount        int64  `json:"fail_count"`
	Quota            int64  `json:"quota"`
	RefundQuota      int64  `json:"refund_quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalUseTime     int64  `json:"total_use_time"`
	ActiveDays       int64  `json:"active_days"`
	LastActiveDate   string `json:"last_active_date"`
}

func QueryUsageUserTable(startDate, endDate string, userIds []int) ([]UsageUserTableRow, error) {
	rows := []UsageUserTableRow{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatDaily{}).
			Select("user_id, MAX(username) AS username, SUM(request_count) AS request_count, "+
				"SUM(fail_count) AS fail_count, SUM(quota) AS quota, SUM(refund_quota) AS refund_quota, "+
				"SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens, "+
				"SUM(total_use_time) AS total_use_time, "+
				"COUNT(DISTINCT CASE WHEN request_count > 0 THEN date END) AS active_days, "+
				// COALESCE 不可省: 范围内全是失败日志(无 consume)的用户 MAX 为 NULL,
				// 直接扫 Go string 会报错。
				"COALESCE(MAX(CASE WHEN request_count > 0 THEN date END), '') AS last_active_date").
			Where("date >= ? AND date <= ?", startDate, endDate).
			Group("user_id")
	})
	return rows, err
}

// UsageUserModelRow 按 用户x模型 聚合,调用方在 Go 侧取每用户主力模型 top1
// (SQL 窗口函数四方言不兼容,不上 SQL 侧 top1)。
type UsageUserModelRow struct {
	UserId       int    `json:"user_id"`
	ModelName    string `json:"model_name"`
	RequestCount int64  `json:"request_count"`
	Quota        int64  `json:"quota"`
}

func QueryUsageByUserModel(startDate, endDate string, userIds []int) ([]UsageUserModelRow, error) {
	rows := []UsageUserModelRow{}
	err := queryUsageStatsInChunks(userIds, &rows, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&UsageStatDaily{}).
			Select("user_id, model_name, SUM(request_count) AS request_count, SUM(quota) AS quota").
			Where("date >= ? AND date <= ?", startDate, endDate).
			Group("user_id, model_name")
	})
	return rows, err
}
