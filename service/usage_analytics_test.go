package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAnalyticsOrg 造一棵三层部门树 + 兄弟分支:
//
//	dept 1(根) ── dept 2 ── dept 3
//	           └── dept 4
//
// 成员: leader1 管 dept 1,leader2 管 dept 2,u-a 在 dept 2,u-b 在 dept 3,
// u-c 在 dept 4,u-unbound 在 dept 2 未绑定。
func seedAnalyticsOrg(t *testing.T) map[string]*model.User {
	t.Helper()
	names := []string{"leader1", "leader2", "u-a", "u-b", "u-c"}
	users := map[string]*model.User{}
	for i, name := range names {
		user := &model.User{
			Username:    name,
			DisplayName: name,
			DingTalkId:  "union-" + name,
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			AffCode:     "aff-analytics-" + name,
		}
		require.NoError(t, model.DB.Create(user).Error)
		users[name] = user
		_ = i
	}
	depts := []model.OrgDepartment{
		{Provider: model.OrgProviderDingTalk, DeptId: "1", ParentId: "", Name: "全体成员"},
		{Provider: model.OrgProviderDingTalk, DeptId: "2", ParentId: "1", Name: "研发部"},
		{Provider: model.OrgProviderDingTalk, DeptId: "3", ParentId: "2", Name: "平台组"},
		{Provider: model.OrgProviderDingTalk, DeptId: "4", ParentId: "1", Name: "市场部"},
	}
	for _, d := range depts {
		require.NoError(t, model.DB.Create(&d).Error)
	}
	members := []model.OrgMember{
		{Provider: model.OrgProviderDingTalk, UnionId: "union-leader1", Name: "主管一", UserId: users["leader1"].Id, DeptIds: `["1"]`, LeaderDeptIds: `["1"]`},
		{Provider: model.OrgProviderDingTalk, UnionId: "union-leader2", Name: "主管二", UserId: users["leader2"].Id, DeptIds: `["2"]`, LeaderDeptIds: `["2"]`},
		{Provider: model.OrgProviderDingTalk, UnionId: "union-u-a", Name: "甲", UserId: users["u-a"].Id, DeptIds: `["2"]`, LeaderDeptIds: `[]`},
		{Provider: model.OrgProviderDingTalk, UnionId: "union-u-b", Name: "乙", UserId: users["u-b"].Id, DeptIds: `["3"]`, LeaderDeptIds: `[]`},
		{Provider: model.OrgProviderDingTalk, UnionId: "union-u-c", Name: "丙", UserId: users["u-c"].Id, DeptIds: `["4"]`, LeaderDeptIds: `[]`},
		{Provider: model.OrgProviderDingTalk, UnionId: "union-unbound", Name: "未绑定", UserId: 0, DeptIds: `["2"]`, LeaderDeptIds: `[]`},
	}
	for _, m := range members {
		require.NoError(t, model.DB.Create(&m).Error)
	}
	return users
}

func enableDingTalk(t *testing.T) {
	t.Helper()
	settings := system_setting.GetDingTalkSettings()
	original := settings.Enabled
	settings.Enabled = true
	t.Cleanup(func() { settings.Enabled = original })
	InvalidateAnalyticsScopeCache()
	t.Cleanup(InvalidateAnalyticsScopeCache)
}

func sortedInts(ids []int) []int {
	out := append([]int{}, ids...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// 管理员直接全量,不碰 org 表(快照为空也能解析)。
func TestResolveAnalyticsScope_Admin(t *testing.T) {
	truncate(t)
	scope, err := ResolveAnalyticsScope(1, common.RoleAdminUser)
	require.NoError(t, err)
	assert.Equal(t, "admin", scope.Scope)
	assert.Nil(t, scope.UserIds)
}

// 根部门负责人看到整棵树;子部门负责人看不到兄弟子树;未绑定成员不进集合。
func TestResolveAnalyticsScope_DeptLeader(t *testing.T) {
	truncate(t)
	enableDingTalk(t)
	users := seedAnalyticsOrg(t)

	// 根主管: 全部已绑定成员(leader1/leader2/u-a/u-b/u-c)。
	scope, err := ResolveAnalyticsScope(users["leader1"].Id, common.RoleCommonUser)
	require.NoError(t, err)
	assert.Equal(t, "dept", scope.Scope)
	assert.Equal(t, sortedInts([]int{
		users["leader1"].Id, users["leader2"].Id, users["u-a"].Id, users["u-b"].Id, users["u-c"].Id,
	}), sortedInts(scope.UserIds))
	assert.ElementsMatch(t, []string{"1", "2", "3", "4"}, scope.DeptIds)
	assert.Equal(t, "研发部", scope.DeptNames["2"])

	// 研发部主管: 自己 + u-a + u-b(dept 3 是下级),不含市场部的 u-c,不含未绑定成员。
	scope, err = ResolveAnalyticsScope(users["leader2"].Id, common.RoleCommonUser)
	require.NoError(t, err)
	assert.Equal(t, sortedInts([]int{users["leader2"].Id, users["u-a"].Id, users["u-b"].Id}), sortedInts(scope.UserIds))
	assert.ElementsMatch(t, []string{"2", "3"}, scope.DeptIds)
	assert.Equal(t, "3", scope.PrimaryDept[users["u-b"].Id])
	assert.Equal(t, "2", scope.PrimaryDept[users["u-a"].Id])
}

// 非负责人/无 provider/未绑定 unionId 一律 forbidden。
func TestResolveAnalyticsScope_Forbidden(t *testing.T) {
	truncate(t)
	enableDingTalk(t)
	users := seedAnalyticsOrg(t)

	// 普通成员不是任何部门主管。
	_, err := ResolveAnalyticsScope(users["u-a"].Id, common.RoleCommonUser)
	assert.ErrorIs(t, err, ErrAnalyticsForbidden)

	// 没绑钉钉账号的用户。
	plain := &model.User{Username: "plain", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "aff-plain"}
	require.NoError(t, model.DB.Create(plain).Error)
	_, err = ResolveAnalyticsScope(plain.Id, common.RoleCommonUser)
	assert.ErrorIs(t, err, ErrAnalyticsForbidden)

	// 关掉企业 IM 后连负责人也无权(快照推导的前提消失)。
	system_setting.GetDingTalkSettings().Enabled = false
	InvalidateAnalyticsScopeCache()
	_, err = ResolveAnalyticsScope(users["leader1"].Id, common.RoleCommonUser)
	assert.ErrorIs(t, err, ErrAnalyticsForbidden)
}

// planAggregationDays: 空表 = 回填 backfillDays 天 + 今天; 全量已聚合 = 昨天 + 今天;
// 不含今天时只剩昨天; backfillDays=0 时不回填。
func TestPlanUsageAggregationDays(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 30, 0, 0, time.Local)

	days := PlanUsageAggregationDays(now, 90, true, map[string]bool{})
	require.Len(t, days, 91) // 90 天回填(含昨天) + 今天
	assert.Equal(t, "2026-08-31", days[len(days)-2].Date)
	assert.Equal(t, "2026-09-01", days[len(days)-1].Date)
	assert.Equal(t, days[len(days)-1].End, now.Unix())
	// 窗口首尾对齐本地零点。
	first := days[0]
	assert.Equal(t, "2026-06-03", first.Date)
	assert.Equal(t, int64(86400), first.End-first.Start)

	// 全部 90 天都已聚合: 只剩重算昨天 + 今天。
	existing := map[string]bool{}
	for d := 0; d < 90; d++ {
		existing[time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, -d-1).Format("2006-01-02")] = true
	}
	days = PlanUsageAggregationDays(now, 90, true, existing)
	require.Len(t, days, 2)
	assert.Equal(t, "2026-08-31", days[0].Date) // 昨天重算
	assert.Equal(t, "2026-09-01", days[1].Date)

	days = PlanUsageAggregationDays(now, 90, false, existing)
	require.Len(t, days, 1)
	assert.Equal(t, "2026-08-31", days[0].Date)

	// 部分缺口: 缺两天补两天,昨天仍重算。
	partial := map[string]bool{}
	for d := 0; d < 90; d++ {
		partial[time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, -d-1).Format("2006-01-02")] = true
	}
	delete(partial, "2026-08-15")
	delete(partial, "2026-08-16")
	delete(partial, "2026-08-31")
	days = PlanUsageAggregationDays(now, 90, true, partial)
	require.Len(t, days, 4) // 缺口三天(含昨天) + 今天
	assert.Equal(t, "2026-08-15", days[0].Date)
	assert.Equal(t, "2026-08-16", days[1].Date)
	assert.Equal(t, "2026-08-31", days[2].Date)
	assert.Equal(t, "2026-09-01", days[3].Date)

	days = PlanUsageAggregationDays(now, 0, false, map[string]bool{})
	require.Len(t, days, 1)
	assert.Equal(t, "2026-08-31", days[0].Date)
}

// 部门归因: 多部门成员按 DeptIds[0] 计主部门,未绑定成员不计数。
func TestLoadOrgDeptAttribution(t *testing.T) {
	truncate(t)
	users := seedAnalyticsOrg(t)

	attribution, err := LoadOrgDeptAttribution(model.OrgProviderDingTalk)
	require.NoError(t, err)
	assert.Equal(t, "市场部", attribution.DeptNames["4"])
	assert.Equal(t, 2, attribution.MemberCount["2"]) // leader2 + u-a, 未绑定不算
	assert.Equal(t, 1, attribution.MemberCount["3"])
	assert.Equal(t, "3", attribution.PrimaryDept[users["u-b"].Id])
	assert.Len(t, attribution.BoundUserIds, 5)
}
