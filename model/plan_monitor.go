package model

import (
	"time"

	"gorm.io/gorm"
)

// PlanMonitor 套餐监控配置:一个上游供应商的一个用量套餐。
// 与渠道(Channel)完全解耦,纯外挂监控,不参与分发/计费。
type PlanMonitor struct {
	Id                 int64  `json:"id" gorm:"primary_key"`
	Provider           string `json:"provider" gorm:"type:varchar(64);index"` // 供应商标识,如 minimax
	PlanName           string `json:"plan_name" gorm:"type:varchar(128)"`     // 套餐名称
	ApiUrl             string `json:"api_url" gorm:"type:varchar(255)"`       // 查询用量接口的 base url
	ApiKey             string `json:"api_key" gorm:"type:text"`               // 查询凭证,返回前端前脱敏
	RefreshIntervalMin int    `json:"refresh_interval_min" gorm:"default:5"`  // 刷新间隔(分钟)
	SortOrder          int    `json:"sort_order" gorm:"default:0"`            // 排序权重,越小越靠前(分组顺序取组内最小值)
	Enabled            bool   `json:"enabled" gorm:"default:true"`
	CreatedTime        int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime        int64  `json:"updated_time" gorm:"bigint"`
	LastFetchTime      int64  `json:"last_fetch_time" gorm:"bigint"` // 最近一次成功拉取时间
	LastError          string `json:"last_error" gorm:"type:text"`   // 最近一次拉取错误,成功时清空

	// usages 不建外键级联,查询时按 plan_id 关联
	Usages []PlanMonitorUsage `json:"usages,omitempty" gorm:"-"`
}

func (PlanMonitor) TableName() string { return "plan_monitors" }

// MaskApiKey 返回脱敏后的 key:长度>8 显示前4后4,否则全脱敏。
func (p *PlanMonitor) MaskApiKey() string {
	k := p.ApiKey
	if len(k) <= 8 {
		if k == "" {
			return ""
		}
		return "********"
	}
	return k[:4] + "****" + k[len(k)-4:]
}

// PlanMonitorUsage 套餐某统计周期的最新用量快照。PlanId+Period 唯一,每次拉取覆盖。
type PlanMonitorUsage struct {
	PlanId           int64   `json:"plan_id" gorm:"primaryKey;autoIncrement:false"`
	Period           string  `json:"period" gorm:"type:varchar(16);primaryKey;autoIncrement:false"` // 5h / weekly / monthly
	UsedPercent      float64 `json:"used_percent"`                                                  // 已用百分比(0-100)
	RemainingPercent float64 `json:"remaining_percent"`                                             // 剩余百分比(0-100)
	PeriodEndTime    int64   `json:"period_end_time" gorm:"bigint"`                                 // 周期截止(重置)时间戳(秒)
	FetchedAt        int64   `json:"fetched_at" gorm:"bigint"`                                      // 本次拉取时间
}

func (PlanMonitorUsage) TableName() string { return "plan_monitor_usages" }

// 统计周期常量
const (
	PlanPeriod5Hour   = "5h"
	PlanPeriodWeekly  = "weekly"
	PlanPeriodMonthly = "monthly"
)

// ---------- 配置 CRUD ----------

func CreatePlanMonitor(p *PlanMonitor) error {
	now := time.Now().Unix()
	p.CreatedTime = now
	p.UpdatedTime = now
	return DB.Create(p).Error
}

func UpdatePlanMonitor(p *PlanMonitor) error {
	p.UpdatedTime = time.Now().Unix()
	// 用 map 更新,避免 bool/int 零值被 GORM 忽略(Enabled=false、RefreshIntervalMin 等)
	return DB.Model(&PlanMonitor{}).Where("id = ?", p.Id).Updates(map[string]interface{}{
		"provider":             p.Provider,
		"plan_name":            p.PlanName,
		"api_url":              p.ApiUrl,
		"api_key":              p.ApiKey,
		"refresh_interval_min": p.RefreshIntervalMin,
		"sort_order":           p.SortOrder,
		"enabled":              p.Enabled,
		"updated_time":         p.UpdatedTime,
	}).Error
}

func GetPlanMonitorById(id int64) (*PlanMonitor, error) {
	var p PlanMonitor
	err := DB.Where("id = ?", id).First(&p).Error
	return &p, err
}

func GetAllPlanMonitors() ([]*PlanMonitor, error) {
	var plans []*PlanMonitor
	// 先按排序权重(小的在前),再按供应商与 id 稳定排序,保证分组与组内顺序都可控。
	err := DB.Order("sort_order asc, provider asc, id asc").Find(&plans).Error
	return plans, err
}

func GetEnabledPlanMonitors() ([]*PlanMonitor, error) {
	var plans []*PlanMonitor
	err := DB.Where("enabled = ?", true).Find(&plans).Error
	return plans, err
}

func DeletePlanMonitor(id int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("plan_id = ?", id).Delete(&PlanMonitorUsage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&PlanMonitor{}).Error
	})
}

// RecordPlanMonitorFetchResult 记录一次拉取结果。成功时清错误并写成功时间;失败只记错误,不动旧快照。
func RecordPlanMonitorFetchResult(id int64, fetchErr error) error {
	updates := map[string]interface{}{"updated_time": time.Now().Unix()}
	if fetchErr != nil {
		updates["last_error"] = fetchErr.Error()
	} else {
		updates["last_error"] = ""
		updates["last_fetch_time"] = time.Now().Unix()
	}
	return DB.Model(&PlanMonitor{}).Where("id = ?", id).Updates(updates).Error
}

// ---------- 用量快照 ----------

// UpsertPlanMonitorUsages 覆盖写某套餐的各周期最新快照(全量替换该套餐的旧快照)。
func UpsertPlanMonitorUsages(planId int64, usages []PlanMonitorUsage) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("plan_id = ?", planId).Delete(&PlanMonitorUsage{}).Error; err != nil {
			return err
		}
		if len(usages) == 0 {
			return nil
		}
		return tx.Create(&usages).Error
	})
}

// GetPlanMonitorUsages 取某套餐的全部周期快照。
func GetPlanMonitorUsages(planId int64) ([]PlanMonitorUsage, error) {
	var usages []PlanMonitorUsage
	err := DB.Where("plan_id = ?", planId).Find(&usages).Error
	return usages, err
}
