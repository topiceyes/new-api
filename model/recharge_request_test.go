package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedRechargeChain 申请人 applicant + 两级审批人(第一级或签两人)。
func seedRechargeChain(t *testing.T) (applicant *User, chain [][]ResolvedRechargeApprover) {
	t.Helper()
	applicant = &User{Username: "applicant", Quota: 100, AffCode: "applicant"}
	require.NoError(t, DB.Create(applicant).Error)
	leader1 := &User{Username: "leader1", AffCode: "leader1"}
	leader2 := &User{Username: "leader2", AffCode: "leader2"}
	boss := &User{Username: "boss", AffCode: "boss"}
	require.NoError(t, DB.Create(leader1).Error)
	require.NoError(t, DB.Create(leader2).Error)
	require.NoError(t, DB.Create(boss).Error)
	chain = [][]ResolvedRechargeApprover{
		{
			{UserId: leader1.Id, UnionId: "u-l1", Name: "L1", LevelType: "dept_leader"},
			{UserId: leader2.Id, UnionId: "u-l2", Name: "L2", LevelType: "dept_leader"},
		},
		{
			{UserId: boss.Id, Name: "Boss", LevelType: "designated"},
		},
	}
	return applicant, chain
}

func newTestRechargeRequest(applicant *User) *RechargeRequest {
	return &RechargeRequest{
		UserId:    applicant.Id,
		Username:  applicant.Username,
		Category:  RechargeCategoryProjectDelivery,
		Detail:    "项目交付需要",
		AmountUsd: 10,
		Quota:     5_000_000,
	}
}

// 全链通过:或签第一级任一人通过即推进,终审后额度恰好增加一次,日志落账。
func TestRechargeFullApprovalCreditsQuotaOnce(t *testing.T) {
	truncateTables(t)
	applicant, chain := seedRechargeChain(t)

	req := newTestRechargeRequest(applicant)
	require.NoError(t, CreateRechargeRequest(req, chain))
	assert.Equal(t, RechargeRequestStatusPending, req.Status)
	assert.Equal(t, 1, req.CurrentLevel)
	assert.Equal(t, 2, req.TotalLevels)

	var stepCount int64
	DB.Model(&RechargeApprovalStep{}).Where("request_id = ?", req.Id).Count(&stepCount)
	assert.EqualValues(t, 3, stepCount)

	// 越级:第二级审批人在第一级未通过时不能审批
	_, _, _, _, err := ApproveRechargeStep(req.Id, chain[1][0].UserId, "")
	require.ErrorIs(t, err, ErrRechargeNotApprover)

	// 第一级或签:leader1 通过,leader2 的步骤应变 skipped
	_, _, final, _, err := ApproveRechargeStep(req.Id, chain[0][0].UserId, "同意")
	require.NoError(t, err)
	assert.False(t, final)
	var peer RechargeApprovalStep
	require.NoError(t, DB.Where("request_id = ? AND level = 1 AND user_id = ?", req.Id, chain[0][1].UserId).First(&peer).Error)
	assert.Equal(t, RechargeStepSkipped, peer.Decision)

	// 终审通过
	req, _, final, _, err = ApproveRechargeStep(req.Id, chain[1][0].UserId, "")
	require.NoError(t, err)
	assert.True(t, final)
	assert.Equal(t, RechargeRequestStatusApproved, req.Status)

	updated, err := GetUserById(applicant.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 100+5_000_000, updated.Quota)

	// 幂等:重复 approve 不再加额度
	_, _, _, alreadyDone, err := ApproveRechargeStep(req.Id, chain[1][0].UserId, "")
	require.ErrorIs(t, err, ErrRechargeRequestStatusInvalid)
	assert.False(t, alreadyDone)
	updated, err = GetUserById(applicant.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 100+5_000_000, updated.Quota)
}

// 拒绝即终止:任一级拒绝后状态 rejected,后续审批一律报状态错误,额度不变。
func TestRechargeRejectTerminates(t *testing.T) {
	truncateTables(t)
	applicant, chain := seedRechargeChain(t)

	req := newTestRechargeRequest(applicant)
	require.NoError(t, CreateRechargeRequest(req, chain))

	_, _, _, err := RejectRechargeStep(req.Id, chain[0][1].UserId, "预算不足")
	require.NoError(t, err)

	updated, err := GetUserById(applicant.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 100, updated.Quota)

	stored, err := GetRechargeRequestById(req.Id)
	require.NoError(t, err)
	assert.Equal(t, RechargeRequestStatusRejected, stored.Status)
	assert.Equal(t, "预算不足", stored.RejectReason)

	_, _, _, _, err = ApproveRechargeStep(req.Id, chain[1][0].UserId, "")
	require.ErrorIs(t, err, ErrRechargeRequestStatusInvalid)
}

// 非审批人不能操作。
func TestRechargeDecideByNonApprover(t *testing.T) {
	truncateTables(t)
	applicant, chain := seedRechargeChain(t)

	req := newTestRechargeRequest(applicant)
	require.NoError(t, CreateRechargeRequest(req, chain))

	_, _, _, err := RejectRechargeStep(req.Id, applicant.Id, "自己拒自己")
	require.ErrorIs(t, err, ErrRechargeNotApprover)
	_, _, _, _, err = ApproveRechargeStep(req.Id, 99999, "")
	require.ErrorIs(t, err, ErrRechargeNotApprover)
}

// 同一用户同时只能有一条 pending 申请。
func TestRechargePendingGuard(t *testing.T) {
	truncateTables(t)
	applicant, chain := seedRechargeChain(t)

	require.NoError(t, CreateRechargeRequest(newTestRechargeRequest(applicant), chain))
	require.ErrorIs(t, CreateRechargeRequest(newTestRechargeRequest(applicant), chain), ErrRechargePendingExists)
}

// 终审时若用户额度顶到钱包上限,整个审批回滚,申请保持 pending 可重试。
func TestRechargeFinalApproveQuotaCeilingRollback(t *testing.T) {
	truncateTables(t)
	applicant := &User{Username: "rich", Quota: common.MaxQuota - 10, AffCode: "rich"}
	require.NoError(t, DB.Create(applicant).Error)
	boss := &User{Username: "boss", AffCode: "boss"}
	require.NoError(t, DB.Create(boss).Error)

	chain := [][]ResolvedRechargeApprover{
		{{UserId: boss.Id, Name: "Boss", LevelType: "designated"}},
	}
	req := newTestRechargeRequest(applicant)
	require.NoError(t, CreateRechargeRequest(req, chain))

	_, _, _, _, err := ApproveRechargeStep(req.Id, boss.Id, "")
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)

	stored, err := GetRechargeRequestById(req.Id)
	require.NoError(t, err)
	assert.Equal(t, RechargeRequestStatusPending, stored.Status)
	assert.Equal(t, 1, stored.CurrentLevel)

	// 步骤也应回滚为 pending,可以重试
	var step RechargeApprovalStep
	require.NoError(t, DB.Where("request_id = ? AND level = 1", req.Id).First(&step).Error)
	assert.Equal(t, RechargeStepPending, step.Decision)
}

// 审批任务列表:待办只含轮到我的,已办含我已通过的。
func TestGetRechargeApprovalTasks(t *testing.T) {
	truncateTables(t)
	applicant, chain := seedRechargeChain(t)

	req := newTestRechargeRequest(applicant)
	require.NoError(t, CreateRechargeRequest(req, chain))

	pageInfo := &common.PageInfo{Page: 1, PageSize: 20}

	// 第一级两人都有待办
	tasks, total, err := GetRechargeApprovalTasks(chain[0][0].UserId, true, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.True(t, tasks[0].IsActionable)

	// 第二级暂无待办(还没轮到)
	_, total, err = GetRechargeApprovalTasks(chain[1][0].UserId, true, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)

	// 第一级通过后,第二级有待办;第一级的人进已办
	_, _, _, _, err = ApproveRechargeStep(req.Id, chain[0][0].UserId, "")
	require.NoError(t, err)
	_, total, err = GetRechargeApprovalTasks(chain[1][0].UserId, true, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)

	decided, total, err := GetRechargeApprovalTasks(chain[0][0].UserId, false, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, RechargeStepApproved, decided[0].MyDecision)
}
