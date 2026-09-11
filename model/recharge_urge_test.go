package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 催批状态机:仅本人、仅 pending、上限 3 次、10 分钟冷却。
func TestRechargeUrgeStateMachine(t *testing.T) {
	truncateTables(t)
	applicant, chain := seedRechargeChain(t)

	req := newTestRechargeRequest(applicant)
	require.NoError(t, CreateRechargeRequest(req, chain))

	// 非申请人不能催
	_, err := UrgeRechargeRequest(req.Id, chain[0][0].UserId)
	require.ErrorIs(t, err, ErrRechargeUrgeNotApplicant)

	// 首次催批成功
	urged, err := UrgeRechargeRequest(req.Id, applicant.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, urged.UrgeCount)
	assert.Greater(t, urged.LastUrgeTime, int64(0))

	// 冷却期内再次催批被拒
	_, err = UrgeRechargeRequest(req.Id, applicant.Id)
	var cooldown *ErrRechargeUrgeCooldown
	require.ErrorAs(t, err, &cooldown)
	assert.Equal(t, RechargeUrgeCooldownSeconds, cooldown.RetryAfterSeconds)

	// 模拟冷却结束,继续催到上限
	assert.NoError(t, DB.Model(&RechargeRequest{}).Where("id = ?", req.Id).
		Update("last_urge_time", 0).Error)
	_, err = UrgeRechargeRequest(req.Id, applicant.Id)
	require.NoError(t, err)
	assert.NoError(t, DB.Model(&RechargeRequest{}).Where("id = ?", req.Id).
		Update("last_urge_time", 0).Error)
	_, err = UrgeRechargeRequest(req.Id, applicant.Id)
	require.NoError(t, err)

	// 第 4 次达到上限
	assert.NoError(t, DB.Model(&RechargeRequest{}).Where("id = ?", req.Id).
		Update("last_urge_time", 0).Error)
	_, err = UrgeRechargeRequest(req.Id, applicant.Id)
	require.ErrorIs(t, err, ErrRechargeUrgeLimit)

	// 申请结束后不能催
	assert.NoError(t, DB.Model(&RechargeRequest{}).Where("id = ?", req.Id).
		Update("urge_count", 0).Error)
	_, _, _, _, err = ApproveRechargeStep(req.Id, chain[0][0].UserId, "")
	require.NoError(t, err)
	_, _, _, _, err = ApproveRechargeStep(req.Id, chain[1][0].UserId, "")
	require.NoError(t, err)
	_, err = UrgeRechargeRequest(req.Id, applicant.Id)
	require.ErrorIs(t, err, ErrRechargeRequestStatusInvalid)

	// 不存在的申请
	_, err = UrgeRechargeRequest(99999, applicant.Id)
	require.ErrorIs(t, err, ErrRechargeRequestNotFound)
}
