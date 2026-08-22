package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 主管分组映射的核心不变量:
// - 新任主管进目标组并记录原分组;已在目标组只标记不写
// - 卸任主管只在当前组仍是目标组时恢复,期间被管理员手动调组就不碰
// - 未绑定本地账号的成员永远不动分组
// - 快照里消失的已映射成员按卸任处理
func TestComputeOrgGroupChanges(t *testing.T) {
	t.Parallel()

	const target = "vip"
	leader := `["2"]`
	plain := `[]`

	t.Run("new leader is mapped and previous group recorded", func(t *testing.T) {
		t.Parallel()
		members := []model.OrgMember{{UnionId: "u1", UserId: 1, LeaderDeptIds: leader}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(members, nil, map[string]bool{"u1": true},
			map[int]string{1: "default"}, target, result)
		require.Len(t, changes, 1)
		assert.Equal(t, model.OrgGroupChange{UserId: 1, ExpectCurrent: "default", NewGroup: "vip"}, changes[0])
		assert.True(t, members[0].GroupMapped)
		assert.Equal(t, "default", members[0].PrevGroup)
		assert.Equal(t, 1, result.GroupMapped)
	})

	t.Run("new leader already in target group is only flagged", func(t *testing.T) {
		t.Parallel()
		members := []model.OrgMember{{UnionId: "u1", UserId: 1, LeaderDeptIds: leader}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(members, nil, map[string]bool{"u1": true},
			map[int]string{1: "vip"}, target, result)
		assert.Empty(t, changes)
		assert.True(t, members[0].GroupMapped)
		assert.Equal(t, "vip", members[0].PrevGroup)
		assert.Equal(t, 0, result.GroupMapped)
	})

	t.Run("still-leader keeps mapping state without changes", func(t *testing.T) {
		t.Parallel()
		states := map[string]model.OrgMember{"u1": {UnionId: "u1", UserId: 1, GroupMapped: true, PrevGroup: "default"}}
		members := []model.OrgMember{{UnionId: "u1", UserId: 1, LeaderDeptIds: leader}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(members, states, map[string]bool{"u1": true},
			map[int]string{1: "vip"}, target, result)
		assert.Empty(t, changes)
		assert.True(t, members[0].GroupMapped)
		assert.Equal(t, "default", members[0].PrevGroup)
	})

	t.Run("demoted leader is restored to previous group", func(t *testing.T) {
		t.Parallel()
		states := map[string]model.OrgMember{"u1": {UnionId: "u1", UserId: 1, GroupMapped: true, PrevGroup: "default"}}
		members := []model.OrgMember{{UnionId: "u1", UserId: 1, LeaderDeptIds: plain}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(members, states, map[string]bool{"u1": true},
			map[int]string{1: "vip"}, target, result)
		require.Len(t, changes, 1)
		assert.Equal(t, model.OrgGroupChange{UserId: 1, ExpectCurrent: "vip", NewGroup: "default"}, changes[0])
		assert.False(t, members[0].GroupMapped)
		assert.Equal(t, 1, result.GroupUnmapped)
	})

	t.Run("demoted leader manually regrouped meanwhile is left alone", func(t *testing.T) {
		t.Parallel()
		states := map[string]model.OrgMember{"u1": {UnionId: "u1", UserId: 1, GroupMapped: true, PrevGroup: "default"}}
		members := []model.OrgMember{{UnionId: "u1", UserId: 1, LeaderDeptIds: plain}}
		result := &OrgSyncResult{}
		// 管理员在两次同步之间把他调到了 svip:状态清除但分组不动。
		changes := computeOrgGroupChanges(members, states, map[string]bool{"u1": true},
			map[int]string{1: "svip"}, target, result)
		assert.Empty(t, changes)
		assert.False(t, members[0].GroupMapped)
		assert.Equal(t, 0, result.GroupUnmapped)
	})

	t.Run("unbound leader is never mapped", func(t *testing.T) {
		t.Parallel()
		members := []model.OrgMember{{UnionId: "u1", UserId: 0, LeaderDeptIds: leader}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(members, nil, map[string]bool{"u1": true},
			map[int]string{}, target, result)
		assert.Empty(t, changes)
		assert.False(t, members[0].GroupMapped)
	})

	t.Run("mapped member vanished from snapshot is unmapped", func(t *testing.T) {
		t.Parallel()
		states := map[string]model.OrgMember{"u-gone": {UnionId: "u-gone", UserId: 9, GroupMapped: true, PrevGroup: "default"}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(nil, states, map[string]bool{},
			map[int]string{9: "vip"}, target, result)
		require.Len(t, changes, 1)
		assert.Equal(t, model.OrgGroupChange{UserId: 9, ExpectCurrent: "vip", NewGroup: "default"}, changes[0])
		assert.Equal(t, 1, result.GroupUnmapped)
	})

	t.Run("vanished mapped member with empty prev group falls back to default", func(t *testing.T) {
		t.Parallel()
		states := map[string]model.OrgMember{"u-gone": {UnionId: "u-gone", UserId: 9, GroupMapped: true}}
		result := &OrgSyncResult{}
		changes := computeOrgGroupChanges(nil, states, map[string]bool{},
			map[int]string{9: "vip"}, target, result)
		require.Len(t, changes, 1)
		assert.Equal(t, "default", changes[0].NewGroup)
	})
}

func TestOrgMemberIsLeader(t *testing.T) {
	t.Parallel()
	assert.True(t, orgMemberIsLeader(`["2"]`))
	assert.False(t, orgMemberIsLeader(`[]`))
	assert.False(t, orgMemberIsLeader(""))
	assert.False(t, orgMemberIsLeader("not-json"))
}

// 调度开关要求:provider 启用 + orgsync 开关 + 凭证齐备,三者缺一不排期。
func TestOrgSyncScheduleEnabled(t *testing.T) {
	ding := system_setting.GetDingTalkSettings()
	fei := system_setting.GetFeishuSettings()
	savedDing, savedFei := *ding, *fei
	t.Cleanup(func() { *ding = savedDing; *fei = savedFei })

	*ding = system_setting.DingTalkSettings{}
	*fei = system_setting.FeishuSettings{}
	assert.Equal(t, "", ActiveOrgSyncProvider())
	assert.False(t, OrgSyncScheduleEnabled())

	// 钉钉启用但同步开关关 -> 不排期。
	ding.Enabled = true
	ding.AppKey = "k"
	ding.AppSecret = "s"
	assert.Equal(t, model.OrgProviderDingTalk, ActiveOrgSyncProvider())
	assert.False(t, OrgSyncScheduleEnabled())

	ding.OrgSyncEnabled = true
	assert.True(t, OrgSyncScheduleEnabled())

	// 凭证缺失 -> 不排期。
	ding.AppSecret = ""
	assert.False(t, OrgSyncScheduleEnabled())
}

// 互斥前提:钉钉优先;同步间隔非法值回落到 6 小时。
func TestOrgSyncIntervalAndProviderPriority(t *testing.T) {
	ding := system_setting.GetDingTalkSettings()
	fei := system_setting.GetFeishuSettings()
	savedDing, savedFei := *ding, *fei
	t.Cleanup(func() { *ding = savedDing; *fei = savedFei })

	*ding = system_setting.DingTalkSettings{}
	*fei = system_setting.FeishuSettings{}

	fei.Enabled = true
	fei.OrgSyncIntervalHours = 12
	assert.Equal(t, model.OrgProviderFeishu, ActiveOrgSyncProvider())
	assert.Equal(t, 12*time.Hour, OrgSyncInterval())

	// 钉钉启用后优先于飞书(互斥校验保证线上不会真同时开,这里只锁优先级)。
	ding.Enabled = true
	ding.OrgSyncIntervalHours = 0
	assert.Equal(t, model.OrgProviderDingTalk, ActiveOrgSyncProvider())
	assert.Equal(t, 6*time.Hour, OrgSyncInterval())
}
