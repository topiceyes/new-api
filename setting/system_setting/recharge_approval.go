package system_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/setting/config"
)

// 充值申请多级审批流程(全局唯一):固定额度 + 审批级别配置。
// 每级审批人可以是申请人的第 N 级部门主管(按组织架构快照解析)
// 或指定用户;同级多人时任一人通过即可(或签)。

type RechargeApprovalLevel struct {
	Type      string `json:"type"`       // RechargeLevelTypeDeptLeader 或 RechargeLevelTypeDesignated
	DeptLevel int    `json:"dept_level"` // 1-5,仅 dept_leader:1=所在部门主管,2=上一级部门主管...
	UserIds   []int  `json:"user_ids"`   // 仅 designated:指定审批人的本地用户 id
}

const (
	RechargeLevelTypeDeptLeader = "dept_leader"
	RechargeLevelTypeDesignated = "designated"

	RechargeMaxDeptLevel = 5
	RechargeMaxLevels    = 5
)

type RechargeApprovalSettings struct {
	Enabled  bool                    `json:"enabled"`
	QuotaUsd float64                 `json:"quota_usd"` // 固定充值金额(美元),提交申请时快照换算 quota
	Levels   []RechargeApprovalLevel `json:"levels"`    // options 表中存 JSON 字符串
}

var defaultRechargeApprovalSettings = RechargeApprovalSettings{}

func init() {
	config.GlobalConfig.Register("recharge_approval", &defaultRechargeApprovalSettings)
}

func GetRechargeApprovalSettings() *RechargeApprovalSettings {
	return &defaultRechargeApprovalSettings
}

// Validate 校验流程配置本身是否合法;enable 时调用方需保证 Validate 通过。
func (s *RechargeApprovalSettings) Validate() error {
	if s.QuotaUsd <= 0 {
		return fmt.Errorf("recharge quota must be positive")
	}
	if len(s.Levels) == 0 {
		return fmt.Errorf("approval levels cannot be empty")
	}
	if len(s.Levels) > RechargeMaxLevels {
		return fmt.Errorf("approval levels cannot exceed %d", RechargeMaxLevels)
	}
	for i, level := range s.Levels {
		switch level.Type {
		case RechargeLevelTypeDeptLeader:
			if level.DeptLevel < 1 || level.DeptLevel > RechargeMaxDeptLevel {
				return fmt.Errorf("level %d: dept_level must be between 1 and %d", i+1, RechargeMaxDeptLevel)
			}
		case RechargeLevelTypeDesignated:
			if len(level.UserIds) == 0 {
				return fmt.Errorf("level %d: designated approvers cannot be empty", i+1)
			}
		default:
			return fmt.Errorf("level %d: unknown approver type %q", i+1, level.Type)
		}
	}
	return nil
}

// NeedsOrgSnapshot 流程含部门主管层级时依赖组织架构同步数据。
func (s *RechargeApprovalSettings) NeedsOrgSnapshot() bool {
	for _, level := range s.Levels {
		if level.Type == RechargeLevelTypeDeptLeader {
			return true
		}
	}
	return false
}
