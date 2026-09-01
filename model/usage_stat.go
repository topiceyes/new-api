package model

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// UsageStatDaily 使用分析日聚合表(主库)。
// 粒度: 日期 x 用户 x 模型 x 令牌 x 分组 x 渠道。
// 由 usage_aggregate 系统任务从 logs 表(LOG_DB)按天聚合而来,不受日志清理影响。
type UsageStatDaily struct {
	Id        int64  `gorm:"primary_key"`
	Date      string `gorm:"type:varchar(10);uniqueIndex:idx_usd_dim,priority:1;index:idx_usd_date;index:idx_usd_user_date,priority:2"` // 'YYYY-MM-DD' 服务器本地时区
	UserId    int    `gorm:"uniqueIndex:idx_usd_dim,priority:2;index:idx_usd_user_date,priority:1"`
	Username  string `gorm:"type:varchar(64);default:''"`
	ModelName string `gorm:"type:varchar(128);uniqueIndex:idx_usd_dim,priority:3;default:''"`
	TokenName string `gorm:"type:varchar(128);uniqueIndex:idx_usd_dim,priority:4;default:''"`
	UseGroup  string `gorm:"type:varchar(64);uniqueIndex:idx_usd_dim,priority:5;default:''"`
	ChannelId int    `gorm:"uniqueIndex:idx_usd_dim,priority:6;default:0"`

	RequestCount     int   `gorm:"default:0"` // consume 日志数
	FailCount        int   `gorm:"default:0"` // error 日志数
	RefundCount      int   `gorm:"default:0"` // refund 日志数
	Quota            int64 `gorm:"bigint;default:0"`
	RefundQuota      int64 `gorm:"bigint;default:0"`
	PromptTokens     int64 `gorm:"bigint;default:0"`
	CompletionTokens int64 `gorm:"bigint;default:0"`
	TotalUseTime     int64 `gorm:"bigint;default:0"` // sum(use_time), consume only

	UpdatedAt int64 `gorm:"bigint"`
}

func (UsageStatDaily) TableName() string { return "usage_stat_daily" }

// UsageStatHourly 小时级聚合(主库),最小维度,专为 7x24 热力图和部门范围过滤。
type UsageStatHourly struct {
	Id           int64  `gorm:"primary_key"`
	Date         string `gorm:"type:varchar(10);uniqueIndex:idx_ush_dim,priority:1;index"`
	Hour         int    `gorm:"uniqueIndex:idx_ush_dim,priority:2"` // 0-23, 服务器本地时区
	UserId       int    `gorm:"uniqueIndex:idx_ush_dim,priority:3;index"`
	RequestCount int    `gorm:"default:0"` // consume only
	Quota        int64  `gorm:"bigint;default:0"`
}

func (UsageStatHourly) TableName() string { return "usage_stat_hourly" }

// UsageStatDay 聚合水位表: 每个已完成聚合的日期一行(包括零日志日),
// 让补缺回填能跳过"已聚合但没有数据"的日子,而不是每小时重扫空窗口。
type UsageStatDay struct {
	Date      string `gorm:"primaryKey;type:varchar(10)"`
	UpdatedAt int64  `gorm:"bigint"`
}

func (UsageStatDay) TableName() string { return "usage_stat_days" }

// usageStatDailyScanRow 聚合查询的扫描行,type 保留用于 Go 侧分桶。
type usageStatDailyScanRow struct {
	UserId        int
	Username      string
	ModelName     string
	TokenName     string
	UseGroup      string
	ChannelId     int
	Type          int
	Cnt           int64
	SumQuota      int64
	SumPrompt     int64
	SumCompletion int64
	SumUseTime    int64
}

type usageStatHourlyScanRow struct {
	Hour     float64 // MySQL/PG 的 FLOOR 可能返回 decimal,扫 float64 兜底
	UserId   int
	Cnt      int64
	SumQuota int64
}

// dayDimKey 日聚合维度键(不含 type;username 取 MAX,不参与维度,
// 否则用户当天改名会产生两行仅 username 不同的记录,撞上唯一索引)。
type dayDimKey struct {
	UserId    int
	ModelName string
	TokenName string
	UseGroup  string
	ChannelId int
}

// AggregateLogsForDay 从 logs 表(LOG_DB)聚合 [dayStart, dayEnd) 窗口的数据。
// 日界由调用方按服务器本地时区计算。SQL 四方言(SQLite/MySQL/PG/ClickHouse)一致,
// 不写任何数据库日期函数,窗口边界全部由 Go 传入。
func AggregateLogsForDay(ctx context.Context, dayStart, dayEnd int64) ([]UsageStatDaily, []UsageStatHourly, error) {
	date := time.Unix(dayStart, 0).In(time.Local).Format("2006-01-02")

	var dailyRows []usageStatDailyScanRow
	dailySelect := "user_id, MAX(username) AS username, model_name, token_name, " + logGroupCol + " AS use_group, channel_id, type, " +
		"COUNT(*) AS cnt, COALESCE(SUM(quota),0) AS sum_quota, " +
		"COALESCE(SUM(prompt_tokens),0) AS sum_prompt, " +
		"COALESCE(SUM(completion_tokens),0) AS sum_completion, " +
		"COALESCE(SUM(use_time),0) AS sum_use_time"
	err := LOG_DB.WithContext(ctx).Table("logs").
		Select(dailySelect).
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError, LogTypeRefund}).
		Group("user_id, model_name, token_name, " + logGroupCol + ", channel_id, type").
		Scan(&dailyRows).Error
	if err != nil {
		return nil, nil, err
	}

	merged := make(map[dayDimKey]*UsageStatDaily)
	var order []dayDimKey
	for _, r := range dailyRows {
		key := dayDimKey{
			UserId:    r.UserId,
			ModelName: r.ModelName,
			TokenName: r.TokenName,
			UseGroup:  r.UseGroup,
			ChannelId: r.ChannelId,
		}
		row, ok := merged[key]
		if !ok {
			row = &UsageStatDaily{
				Date:      date,
				UserId:    r.UserId,
				Username:  r.Username,
				ModelName: r.ModelName,
				TokenName: r.TokenName,
				UseGroup:  r.UseGroup,
				ChannelId: r.ChannelId,
			}
			merged[key] = row
			order = append(order, key)
		}
		switch r.Type {
		case LogTypeConsume:
			row.RequestCount = int(r.Cnt)
			row.Quota = r.SumQuota
			row.PromptTokens = r.SumPrompt
			row.CompletionTokens = r.SumCompletion
			row.TotalUseTime = r.SumUseTime
		case LogTypeError:
			row.FailCount = int(r.Cnt)
		case LogTypeRefund:
			row.RefundCount = int(r.Cnt)
			row.RefundQuota = r.SumQuota
		}
	}
	daily := make([]UsageStatDaily, 0, len(order))
	for _, key := range order {
		daily = append(daily, *merged[key])
	}

	var hourlyRows []usageStatHourlyScanRow
	// floor 保持小写: ClickHouse 仅对 SQL 标准函数做大小写不敏感,
	// 老版本(21.x 之前)不认识大写 FLOOR; 小写四方言都安全。
	err = LOG_DB.WithContext(ctx).Table("logs").
		Select("floor((created_at - ?) / 3600) AS hour, user_id, COUNT(*) AS cnt, COALESCE(SUM(quota),0) AS sum_quota", dayStart).
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Where("type = ?", LogTypeConsume).
		Group("hour, user_id").
		Scan(&hourlyRows).Error
	if err != nil {
		return nil, nil, err
	}
	hourly := make([]UsageStatHourly, 0, len(hourlyRows))
	for _, r := range hourlyRows {
		hourly = append(hourly, UsageStatHourly{
			Date:         date,
			Hour:         int(r.Hour),
			UserId:       r.UserId,
			RequestCount: int(r.Cnt),
			Quota:        r.SumQuota,
		})
	}

	return daily, hourly, nil
}

// ReplaceUsageStatsForDay 幂等重写某天的聚合结果: 一个事务内先删后插。
func ReplaceUsageStatsForDay(date string, daily []UsageStatDaily, hourly []UsageStatHourly) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date = ?", date).Delete(&UsageStatDaily{}).Error; err != nil {
			return err
		}
		if err := tx.Where("date = ?", date).Delete(&UsageStatHourly{}).Error; err != nil {
			return err
		}
		if len(daily) > 0 {
			// UpdatedAt 由 GORM 按字段名约定自动写当前时间,无需手动赋值。
			if err := tx.CreateInBatches(daily, 100).Error; err != nil {
				return err
			}
		}
		if len(hourly) > 0 {
			if err := tx.CreateInBatches(hourly, 100).Error; err != nil {
				return err
			}
		}
		// 无论当天有没有日志都写水位行,否则零日志日会被补缺机制每小时重扫。
		if err := tx.Save(&UsageStatDay{Date: date}).Error; err != nil {
			return err
		}
		return nil
	})
}

// ListAggregatedDates 返回 sinceDate(含)之后已完成聚合的日期集合(读水位表,
// 零日志日也算已聚合),用于补缺回填。
func ListAggregatedDates(sinceDate string) (map[string]bool, error) {
	var dates []string
	err := DB.Model(&UsageStatDay{}).
		Where("date >= ?", sinceDate).
		Pluck("date", &dates).Error
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(dates))
	for _, d := range dates {
		set[d] = true
	}
	return set, nil
}

// DeleteUsageStatsBefore 清理 date(不含)之前的聚合数据(聚合表自己的保留期,独立于日志清理)。
func DeleteUsageStatsBefore(date string) error {
	if err := DB.Where("date < ?", date).Delete(&UsageStatDaily{}).Error; err != nil {
		return err
	}
	if err := DB.Where("date < ?", date).Delete(&UsageStatHourly{}).Error; err != nil {
		return err
	}
	if err := DB.Where("date < ?", date).Delete(&UsageStatDay{}).Error; err != nil {
		return err
	}
	return nil
}
