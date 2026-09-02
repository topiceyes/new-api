package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedUsageStatDailyRows(t *testing.T, rows []model.UsageStatDaily) {
	t.Helper()
	require.NoError(t, model.DB.CreateInBatches(rows, 100).Error)
}

func entriesByKey(entries []AnalyticsUserTableEntry) map[string]AnalyticsUserTableEntry {
	out := make(map[string]AnalyticsUserTableEntry, len(entries))
	for _, e := range entries {
		out[e.MemberKey] = e
	}
	return out
}

func mustAdminScope(t *testing.T) *AnalyticsScope {
	t.Helper()
	scope, err := ResolveAnalyticsScope(1, common.RoleAdminUser)
	require.NoError(t, err)
	return scope
}

// admin 全集: 活跃/沉默/未绑定三态齐备;有统计但不在名册的用户(root 之类)也在;
// 未绑定成员真名取通讯录、带部门名。
func TestBuildAnalyticsUserTable_AdminUniverse(t *testing.T) {
	truncate(t)
	enableDingTalk(t)
	users := seedAnalyticsOrg(t)

	// u-a 活跃(两天), u-b 只有失败(沉默), leader1/leader2/u-c 无任何统计(沉默)。
	seedUsageStatDailyRows(t, []model.UsageStatDaily{
		{Date: "2026-08-01", UserId: users["u-a"].Id, Username: "u-a", ModelName: "m1", RequestCount: 2, Quota: 100, TotalUseTime: 10},
		{Date: "2026-08-02", UserId: users["u-a"].Id, Username: "u-a", ModelName: "m1", RequestCount: 1, Quota: 50, RefundQuota: 20, TotalUseTime: 2},
		{Date: "2026-08-01", UserId: users["u-b"].Id, Username: "u-b", ModelName: "m1", FailCount: 3},
	})

	entries, err := BuildAnalyticsUserTable(mustAdminScope(t), "2026-08-01", "2026-08-02")
	require.NoError(t, err)
	byKey := entriesByKey(entries)

	// 5 个绑定用户 + 1 个未绑定成员。
	require.Len(t, entries, 6)

	ua := byKey[model2key(users["u-a"].Id)]
	assert.Equal(t, AnalyticsUserStatusActive, ua.Status)
	assert.Equal(t, int64(3), ua.RequestCount)
	assert.Equal(t, int64(130), ua.NetQuota)
	assert.Equal(t, int64(2), ua.ActiveDays)
	assert.Equal(t, "2026-08-02", ua.LastActiveDate)
	assert.Equal(t, "m1", ua.TopModel)
	assert.InDelta(t, 4.0, ua.AvgUseTime, 1e-9) // 12s / 3 次
	assert.Equal(t, "研发部", ua.DeptName)

	ub := byKey[model2key(users["u-b"].Id)]
	assert.Equal(t, AnalyticsUserStatusSilent, ub.Status) // 纯失败不算活跃
	assert.Equal(t, int64(3), ub.FailCount)
	assert.Equal(t, "", ub.LastActiveDate)
	assert.Equal(t, "平台组", ub.DeptName)

	uc := byKey[model2key(users["u-c"].Id)]
	assert.Equal(t, AnalyticsUserStatusSilent, uc.Status)
	assert.Equal(t, "市场部", uc.DeptName)

	unbound := byKey["org:union-unbound"]
	assert.Equal(t, AnalyticsUserStatusNever, unbound.Status)
	assert.Equal(t, 0, unbound.UserId)
	assert.Equal(t, "未绑定", unbound.DisplayName) // 通讯录真名
	assert.Equal(t, "研发部", unbound.DeptName)
	assert.Empty(t, unbound.Username)

	// 快照外账号(本地注册、无 org 绑定)有统计也要在表里,部门为空。
	outsider := &model.User{Username: "root-local", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "aff-outsider"}
	require.NoError(t, model.DB.Create(outsider).Error)
	seedUsageStatDailyRows(t, []model.UsageStatDaily{
		{Date: "2026-08-01", UserId: outsider.Id, Username: "root-local", ModelName: "m9", RequestCount: 1, Quota: 5},
	})
	entries, err = BuildAnalyticsUserTable(mustAdminScope(t), "2026-08-01", "2026-08-02")
	require.NoError(t, err)
	out := entriesByKey(entries)[model2key(outsider.Id)]
	assert.Equal(t, AnalyticsUserStatusActive, out.Status)
	assert.Equal(t, "", out.DeptName)
	assert.Equal(t, "m9", out.TopModel)
}

// 部门负责人: 只看到子树成员(含未绑定),兄弟部门成员与未绑定成员不可见。
func TestBuildAnalyticsUserTable_DeptLeaderUniverse(t *testing.T) {
	truncate(t)
	enableDingTalk(t)
	users := seedAnalyticsOrg(t)

	seedUsageStatDailyRows(t, []model.UsageStatDaily{
		{Date: "2026-08-01", UserId: users["u-a"].Id, Username: "u-a", ModelName: "m1", RequestCount: 1, Quota: 10},
		{Date: "2026-08-01", UserId: users["u-c"].Id, Username: "u-c", ModelName: "m1", RequestCount: 99, Quota: 999},
	})

	scope, err := ResolveAnalyticsScope(users["leader2"].Id, common.RoleCommonUser)
	require.NoError(t, err)
	entries, err := BuildAnalyticsUserTable(scope, "2026-08-01", "2026-08-02")
	require.NoError(t, err)
	byKey := entriesByKey(entries)

	// leader2 + u-a + u-b + 未绑定成员(dept 2);市场部的 u-c 及其统计不可见。
	require.Len(t, entries, 4)
	assert.Contains(t, byKey, model2key(users["leader2"].Id))
	assert.Contains(t, byKey, model2key(users["u-a"].Id))
	assert.Contains(t, byKey, model2key(users["u-b"].Id))
	assert.Contains(t, byKey, "org:union-unbound")
	assert.NotContains(t, byKey, model2key(users["u-c"].Id))

	// 未绑定成员归到子树内部门名。
	assert.Equal(t, "研发部", byKey["org:union-unbound"].DeptName)
	// u-b 的主部门是子树内的 dept 3。
	assert.Equal(t, "平台组", byKey[model2key(users["u-b"].Id)].DeptName)
}

// 未配置组织同步时的兜底: 全集=本地未删用户,只有 active/silent,没有 never。
func TestBuildAnalyticsUserTable_NoProviderFallback(t *testing.T) {
	truncate(t)
	// 不开钉钉/飞书: provider 为空。
	system_setting.GetDingTalkSettings().Enabled = false
	system_setting.GetFeishuSettings().Enabled = false

	users := seedAnalyticsOrg(t)
	seedUsageStatDailyRows(t, []model.UsageStatDaily{
		{Date: "2026-08-01", UserId: users["u-a"].Id, Username: "u-a", ModelName: "m1", RequestCount: 1, Quota: 10},
	})

	entries, err := BuildAnalyticsUserTable(mustAdminScope(t), "2026-08-01", "2026-08-02")
	require.NoError(t, err)
	require.Len(t, entries, 5) // 5 个本地用户,org 快照不参与
	for _, e := range entries {
		assert.NotEqual(t, AnalyticsUserStatusNever, e.Status)
		assert.Equal(t, "", e.DeptName)
	}
	byKey := entriesByKey(entries)
	assert.Equal(t, AnalyticsUserStatusActive, byKey[model2key(users["u-a"].Id)].Status)
	assert.Equal(t, AnalyticsUserStatusSilent, byKey[model2key(users["u-b"].Id)].Status)
}

// 主力模型 top1: 先比请求数,平手比 quota,再平手按模型名字典序,保证确定性。
func TestTopModelPerUserDeterministic(t *testing.T) {
	truncate(t)

	seedUsageStatDailyRows(t, []model.UsageStatDaily{
		// 用户1: m2 请求数更多 -> m2
		{Date: "2026-08-01", UserId: 1, Username: "u1", ModelName: "m1", RequestCount: 1, Quota: 900},
		{Date: "2026-08-01", UserId: 1, Username: "u1", ModelName: "m2", RequestCount: 3, Quota: 100},
		// 用户2: 请求数相同,m2 quota 更高 -> m2
		{Date: "2026-08-01", UserId: 2, Username: "u2", ModelName: "m1", RequestCount: 2, Quota: 100},
		{Date: "2026-08-01", UserId: 2, Username: "u2", ModelName: "m2", RequestCount: 2, Quota: 200},
		// 用户3: 请求数与 quota 都相同,按字典序 -> a-model
		{Date: "2026-08-01", UserId: 3, Username: "u3", ModelName: "b-model", RequestCount: 1, Quota: 100},
		{Date: "2026-08-01", UserId: 3, Username: "u3", ModelName: "a-model", RequestCount: 1, Quota: 100},
		// 用户4: 只有失败日志 -> 无主力模型
		{Date: "2026-08-01", UserId: 4, Username: "u4", ModelName: "m1", FailCount: 5},
	})

	tops, err := topModelPerUser("2026-08-01", "2026-08-01", nil)
	require.NoError(t, err)
	assert.Equal(t, "m2", tops[1])
	assert.Equal(t, "m2", tops[2])
	assert.Equal(t, "a-model", tops[3])
	_, ok := tops[4]
	assert.False(t, ok)
}

func model2key(uid int) string {
	return strconv.Itoa(uid)
}
