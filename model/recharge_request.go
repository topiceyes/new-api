package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"gorm.io/gorm"
)

// 充值申请多级审批的状态机与持久化。审批链在提交时解析并逐行快照到
// recharge_approval_steps,后续组织架构变更不影响在途申请。
// 同级多个审批人采用或签:任一人通过即进入下一级,其余 pending 步骤置为
// skipped;任一人拒绝整个申请立即终止。

const (
	RechargeRequestStatusPending  = "pending"
	RechargeRequestStatusApproved = "approved"
	RechargeRequestStatusRejected = "rejected"
)

const (
	RechargeCategoryProjectDelivery = "project_delivery"
	RechargeCategoryTechResearch    = "tech_research"
)

const (
	RechargeStepPending  = "pending"
	RechargeStepApproved = "approved"
	RechargeStepRejected = "rejected"
	RechargeStepSkipped  = "skipped"
)

var (
	ErrRechargeRequestNotFound      = errors.New("充值申请不存在")
	ErrRechargeRequestStatusInvalid = errors.New("申请状态已变更,请刷新后重试")
	ErrRechargeNotApprover          = errors.New("你不是当前级别的审批人")
	ErrRechargePendingExists        = errors.New("你已有一条待审批的充值申请,请等待审批完成")
)

func IsValidRechargeCategory(category string) bool {
	return category == RechargeCategoryProjectDelivery || category == RechargeCategoryTechResearch
}

type RechargeRequest struct {
	Id           int     `json:"id" gorm:"primary_key"`
	UserId       int     `json:"user_id" gorm:"index"`
	Username     string  `json:"username" gorm:"type:varchar(64)"` // 申请人快照
	Category     string  `json:"category" gorm:"type:varchar(32);index"`
	Detail       string  `json:"detail" gorm:"type:text"`
	AmountUsd    float64 `json:"amount_usd"` // 提交时的流程配置快照
	Quota        int     `json:"quota"`      // 提交时按 QuotaPerUnit 换算的快照
	Status       string  `json:"status" gorm:"type:varchar(16);index"`
	CurrentLevel int     `json:"current_level"` // pending 期间的当前审批级(1 起)
	TotalLevels  int     `json:"total_levels"`
	RejectReason string  `json:"reject_reason" gorm:"type:text"`
	CreateTime   int64   `json:"create_time" gorm:"bigint"`
	CompleteTime int64   `json:"complete_time" gorm:"bigint"`
}

func (RechargeRequest) TableName() string {
	return "recharge_requests"
}

// RechargeApprovalStep 一行 = 某申请某级别的一位审批人,提交时全量创建。
type RechargeApprovalStep struct {
	Id         int64  `json:"id" gorm:"primary_key"`
	RequestId  int    `json:"request_id" gorm:"uniqueIndex:idx_recharge_step,priority:1;index"`
	Level      int    `json:"level" gorm:"uniqueIndex:idx_recharge_step,priority:2"`
	UserId     int    `json:"user_id" gorm:"uniqueIndex:idx_recharge_step,priority:3;index"`
	UnionId    string `json:"union_id" gorm:"type:varchar(128)"` // IM 通知用
	Name       string `json:"name" gorm:"type:varchar(128)"`     // 展示名快照
	LevelType  string `json:"level_type" gorm:"type:varchar(16)"`
	Decision   string `json:"decision" gorm:"type:varchar(16);index"`
	Comment    string `json:"comment" gorm:"type:text"`
	DecideTime int64  `json:"decide_time" gorm:"bigint"`
}

func (RechargeApprovalStep) TableName() string {
	return "recharge_approval_steps"
}

// ResolvedRechargeApprover 服务层解析出的一位审批人。
type ResolvedRechargeApprover struct {
	UserId    int    `json:"user_id"`
	UnionId   string `json:"union_id,omitempty"`
	Name      string `json:"name"`
	LevelType string `json:"level_type"`
}

// CreateRechargeRequest 单事务创建申请与全部审批步骤;同一用户同时只能有
// 一条 pending 申请。
func CreateRechargeRequest(req *RechargeRequest, chain [][]ResolvedRechargeApprover) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// 先锁申请人用户行,串行化同一用户的并发提交,防止下面的
		// count-then-insert 在 MySQL/PG 上被并发双击绕过。
		var applicant User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", req.UserId).First(&applicant).Error; err != nil {
			return err
		}
		var pendingCount int64
		if err := tx.Model(&RechargeRequest{}).
			Where("user_id = ? AND status = ?", req.UserId, RechargeRequestStatusPending).
			Count(&pendingCount).Error; err != nil {
			return err
		}
		if pendingCount > 0 {
			return ErrRechargePendingExists
		}
		req.Status = RechargeRequestStatusPending
		req.CurrentLevel = 1
		req.TotalLevels = len(chain)
		req.CreateTime = common.GetTimestamp()
		if err := tx.Create(req).Error; err != nil {
			return err
		}
		steps := make([]RechargeApprovalStep, 0, len(chain))
		for level, approvers := range chain {
			for _, approver := range approvers {
				steps = append(steps, RechargeApprovalStep{
					RequestId: req.Id,
					Level:     level + 1,
					UserId:    approver.UserId,
					UnionId:   approver.UnionId,
					Name:      approver.Name,
					LevelType: approver.LevelType,
					Decision:  RechargeStepPending,
				})
			}
		}
		return tx.Create(&steps).Error
	})
}

// decideRechargeStep 审批/拒绝的公共骨架:行锁校验 + 当前级 pending 步骤定位。
// decided 返回命中的步骤供后续通知;alreadyDone 表示幂等重复操作。
func decideRechargeStep(tx *gorm.DB, requestId int, approverUserId int) (*RechargeRequest, *RechargeApprovalStep, bool, error) {
	req := &RechargeRequest{}
	if err := lockForUpdate(tx).Where("id = ?", requestId).First(req).Error; err != nil {
		return nil, nil, false, ErrRechargeRequestNotFound
	}
	if req.Status != RechargeRequestStatusPending {
		return nil, nil, false, ErrRechargeRequestStatusInvalid
	}
	step := &RechargeApprovalStep{}
	err := tx.Where("request_id = ? AND level = ? AND user_id = ?", requestId, req.CurrentLevel, approverUserId).First(step).Error
	if err != nil {
		return nil, nil, false, ErrRechargeNotApprover
	}
	if step.Decision != RechargeStepPending {
		return req, step, true, nil
	}
	return req, step, false, nil
}

// skipPeerSteps 同级其余 pending 审批人置为 skipped(或签被抢先/被拒绝时)。
func skipPeerSteps(tx *gorm.DB, requestId int, level int, decidedStepId int64) error {
	return tx.Model(&RechargeApprovalStep{}).
		Where("request_id = ? AND level = ? AND decision = ? AND id <> ?", requestId, level, RechargeStepPending, decidedStepId).
		Update("decision", RechargeStepSkipped).Error
}

// ApproveRechargeStep 审批通过:推进到下一级,最后一级则在同一事务内入账。
// 返回 (申请, 通过的步骤, 是否终审通过, 是否幂等重复)。
func ApproveRechargeStep(requestId int, approverUserId int, comment string) (req *RechargeRequest, step *RechargeApprovalStep, finalApproved bool, alreadyDone bool, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		req, step, alreadyDone, txErr = decideRechargeStep(tx, requestId, approverUserId)
		if txErr != nil || alreadyDone {
			return txErr
		}
		step.Decision = RechargeStepApproved
		step.Comment = comment
		step.DecideTime = common.GetTimestamp()
		if err := tx.Save(step).Error; err != nil {
			return err
		}
		if err := skipPeerSteps(tx, requestId, req.CurrentLevel, step.Id); err != nil {
			return err
		}
		if req.CurrentLevel == req.TotalLevels {
			// 终审:先入账再改状态,creditTopUpQuota 失败(超上限)整体回滚
			if err := creditTopUpQuota(tx, req.UserId, req.Quota, nil); err != nil {
				return err
			}
			req.Status = RechargeRequestStatusApproved
			req.CompleteTime = common.GetTimestamp()
			finalApproved = true
		} else {
			req.CurrentLevel++
		}
		return tx.Save(req).Error
	})
	if err != nil {
		return nil, nil, false, false, err
	}
	if finalApproved {
		syncCreditUserQuotaCache(req.UserId, req.Quota, "recharge approval")
		RecordTopupLog(req.UserId, fmt.Sprintf("充值申请审批通过，到账额度: %v", logger.LogQuota(req.Quota)), "", "approval", "approval")
	}
	return req, step, finalApproved, alreadyDone, nil
}

// RejectRechargeStep 任一级拒绝即终止整个申请。
func RejectRechargeStep(requestId int, approverUserId int, comment string) (req *RechargeRequest, step *RechargeApprovalStep, alreadyDone bool, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		req, step, alreadyDone, txErr = decideRechargeStep(tx, requestId, approverUserId)
		if txErr != nil || alreadyDone {
			return txErr
		}
		step.Decision = RechargeStepRejected
		step.Comment = comment
		step.DecideTime = common.GetTimestamp()
		if err := tx.Save(step).Error; err != nil {
			return err
		}
		if err := skipPeerSteps(tx, requestId, req.CurrentLevel, step.Id); err != nil {
			return err
		}
		req.Status = RechargeRequestStatusRejected
		req.RejectReason = comment
		req.CompleteTime = common.GetTimestamp()
		return tx.Save(req).Error
	})
	if err != nil {
		return nil, nil, false, err
	}
	return req, step, alreadyDone, nil
}

// PendingRechargeSteps 取某申请当前级的全部 pending 审批人(通知用)。
func PendingRechargeSteps(requestId int, level int) ([]RechargeApprovalStep, error) {
	var steps []RechargeApprovalStep
	err := DB.Where("request_id = ? AND level = ? AND decision = ?", requestId, level, RechargeStepPending).
		Find(&steps).Error
	return steps, err
}

func GetRechargeRequestById(id int) (*RechargeRequest, error) {
	req := &RechargeRequest{}
	if err := DB.Where("id = ?", id).First(req).Error; err != nil {
		return nil, err
	}
	return req, nil
}

func GetRechargeRequestSteps(requestId int) ([]RechargeApprovalStep, error) {
	var steps []RechargeApprovalStep
	err := DB.Where("request_id = ?", requestId).Order("level asc, id asc").Find(&steps).Error
	return steps, err
}

// IsRechargeRequestApprover 判断用户是否是某申请任一级的审批人(详情可见性)。
func IsRechargeRequestApprover(requestId int, userId int) bool {
	var count int64
	DB.Model(&RechargeApprovalStep{}).Where("request_id = ? AND user_id = ?", requestId, userId).Count(&count)
	return count > 0
}

func GetUserRechargeRequests(userId int, pageInfo *common.PageInfo) (requests []*RechargeRequest, total int64, err error) {
	if err = DB.Model(&RechargeRequest{}).Where("user_id = ?", userId).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = DB.Where("user_id = ?", userId).Order("id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&requests).Error
	return requests, total, err
}

// RechargeApprovalTask 审批人视角的待办/已办条目。
type RechargeApprovalTask struct {
	RechargeRequest
	MyDecision   string `json:"my_decision"`
	MyComment    string `json:"my_comment"`
	MyLevel      int    `json:"my_level"`
	IsActionable bool   `json:"is_actionable"` // 当前轮到我审批
}

// GetRechargeApprovalTasks 审批人的任务列表:pending=待办(申请 pending 且轮到
// 我所在级),decided=我已处理的(含被同级抢先的 skipped 不展示)。
func GetRechargeApprovalTasks(userId int, pendingOnly bool, pageInfo *common.PageInfo) (tasks []*RechargeApprovalTask, total int64, err error) {
	base := DB.Model(&RechargeRequest{}).
		Select("recharge_requests.*, recharge_approval_steps.decision as my_decision, recharge_approval_steps.comment as my_comment, recharge_approval_steps.level as my_level").
		Joins("JOIN recharge_approval_steps ON recharge_approval_steps.request_id = recharge_requests.id AND recharge_approval_steps.user_id = ?", userId)
	if pendingOnly {
		base = base.Where("recharge_requests.status = ? AND recharge_requests.current_level = recharge_approval_steps.level AND recharge_approval_steps.decision = ?",
			RechargeRequestStatusPending, RechargeStepPending)
	} else {
		base = base.Where("recharge_approval_steps.decision IN ?", []string{RechargeStepApproved, RechargeStepRejected})
	}
	if err = base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*RechargeApprovalTask
	err = base.Order("recharge_requests.id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		row.IsActionable = row.Status == RechargeRequestStatusPending && row.MyLevel == row.CurrentLevel && row.MyDecision == RechargeStepPending
	}
	return rows, total, nil
}

// GetAllRechargeRequests 管理员全量列表,支持状态/事由/申请人关键字过滤。
func GetAllRechargeRequests(status string, category string, keyword string, pageInfo *common.PageInfo) (requests []*RechargeRequest, total int64, err error) {
	base := DB.Model(&RechargeRequest{})
	if status != "" {
		base = base.Where("status = ?", status)
	}
	if category != "" {
		base = base.Where("category = ?", category)
	}
	if keyword != "" {
		base = base.Where("username LIKE ?", "%"+keyword+"%")
	}
	if err = base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = base.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&requests).Error
	return requests, total, err
}
