package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetRechargeSeams 替换审批链解析的全部查找 seam,返回恢复函数由 t.Cleanup 注册。
func resetRechargeSeams(t *testing.T) {
	t.Helper()
	originals := map[string]any{
		"provider":   rechargeActiveOrgProvider,
		"userById":   rechargeGetUserById,
		"member":     rechargeGetOrgMemberByUnion,
		"depts":      rechargeGetOrgDepartments,
		"members":    rechargeGetOrgMembers,
		"unionToUid": rechargeGetUserIdsByUnionIds,
	}
	t.Cleanup(func() {
		rechargeActiveOrgProvider = originals["provider"].(func() string)
		rechargeGetUserById = originals["userById"].(func(int) (*model.User, error))
		rechargeGetOrgMemberByUnion = originals["member"].(func(string, string) (*model.OrgMember, error))
		rechargeGetOrgDepartments = originals["depts"].(func(string) ([]model.OrgDepartment, error))
		rechargeGetOrgMembers = originals["members"].(func(string) ([]model.OrgMember, error))
		rechargeGetUserIdsByUnionIds = originals["unionToUid"].(func(string, []string) (map[string]int, error))
	})
}

// stubOrgSnapshot 三层部门树: 1(全体) -> 2(研发,主管 u-l1) -> 3(后端,主管 u-l2);
// 申请人 u-app 在部门 3;u-l3 是部门 1 的主管但未绑定本地账号。
func stubOrgSnapshot(t *testing.T) {
	t.Helper()
	rechargeActiveOrgProvider = func() string { return model.OrgProviderDingTalk }
	rechargeGetUserById = func(id int) (*model.User, error) {
		users := map[int]*model.User{
			100: {Id: 100, Username: "app", DingTalkId: "u-app"},
			201: {Id: 201, Username: "leader1", DisplayName: "主管一", DingTalkId: "u-l1"},
			202: {Id: 202, Username: "leader2", DingTalkId: "u-l2"},
			301: {Id: 301, Username: "designated", DisplayName: "指定人"},
		}
		if u, ok := users[id]; ok {
			return u, nil
		}
		return nil, nil
	}
	rechargeGetOrgMemberByUnion = func(provider, unionId string) (*model.OrgMember, error) {
		if unionId == "u-app" {
			return &model.OrgMember{
				Provider: provider, UnionId: "u-app", Name: "申请人",
				DeptIds: `["3"]`, LeaderDeptIds: `[]`,
			}, nil
		}
		return nil, nil
	}
	rechargeGetOrgDepartments = func(provider string) ([]model.OrgDepartment, error) {
		return []model.OrgDepartment{
			{Provider: provider, DeptId: "1", ParentId: "", Name: "全体成员"},
			{Provider: provider, DeptId: "2", ParentId: "1", Name: "研发部"},
			{Provider: provider, DeptId: "3", ParentId: "2", Name: "后端组"},
		}, nil
	}
	rechargeGetOrgMembers = func(provider string) ([]model.OrgMember, error) {
		return []model.OrgMember{
			{Provider: provider, UnionId: "u-app", Name: "申请人", DeptIds: `["3"]`, LeaderDeptIds: `[]`},
			{Provider: provider, UnionId: "u-l1", Name: "主管一", DeptIds: `["2"]`, LeaderDeptIds: `["3"]`},
			{Provider: provider, UnionId: "u-l2", Name: "主管二", DeptIds: `["1"]`, LeaderDeptIds: `["2"]`},
			{Provider: provider, UnionId: "u-l3", Name: "主管三", DeptIds: `["1"]`, LeaderDeptIds: `["1"]`},
		}, nil
	}
	// u-l3 没有本地账号绑定
	rechargeGetUserIdsByUnionIds = func(provider string, unionIds []string) (map[string]int, error) {
		all := map[string]int{"u-app": 100, "u-l1": 201, "u-l2": 202}
		result := map[string]int{}
		for _, id := range unionIds {
			if uid, ok := all[id]; ok {
				result[id] = uid
			}
		}
		return result, nil
	}
}

func TestResolveApprovalChainDeptLeaderLevels(t *testing.T) {
	resetRechargeSeams(t)
	stubOrgSnapshot(t)

	chain, err := ResolveApprovalChain(100, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDeptLeader, DeptLevel: 1},
		{Type: system_setting.RechargeLevelTypeDeptLeader, DeptLevel: 2},
	})
	require.NoError(t, err)
	require.Len(t, chain, 2)
	// 第 1 级 = 所在部门(后端组)主管 u-l1
	require.Len(t, chain[0], 1)
	assert.Equal(t, 201, chain[0][0].UserId)
	assert.Equal(t, "主管一", chain[0][0].Name)
	// 第 2 级 = 上一级部门(研发部)主管 u-l2
	require.Len(t, chain[1], 1)
	assert.Equal(t, 202, chain[1][0].UserId)
}

func TestResolveApprovalChainDropsUnboundLeader(t *testing.T) {
	resetRechargeSeams(t)
	stubOrgSnapshot(t)

	// 第 3 级 = 部门 1 的主管 u-l3,但他未绑定本地账号 → 该级为空,解析失败
	_, err := ResolveApprovalChain(100, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDeptLeader, DeptLevel: 3},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "第 1 级审批人解析失败")
}

func TestResolveApprovalChainSelfExclusion(t *testing.T) {
	resetRechargeSeams(t)
	stubOrgSnapshot(t)
	// 申请人自己就是后端组主管
	rechargeGetOrgMemberByUnion = func(provider, unionId string) (*model.OrgMember, error) {
		return &model.OrgMember{
			Provider: provider, UnionId: "u-l1", Name: "主管一",
			DeptIds: `["3"]`, LeaderDeptIds: `["3"]`,
		}, nil
	}
	rechargeGetUserById = func(id int) (*model.User, error) {
		if id == 201 {
			return &model.User{Id: 201, Username: "leader1", DingTalkId: "u-l1"}, nil
		}
		return nil, nil
	}

	// 唯一主管是自己 → 自我排除后该级为空
	_, err := ResolveApprovalChain(201, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDeptLeader, DeptLevel: 1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无法审批")
}

func TestResolveApprovalChainDesignated(t *testing.T) {
	resetRechargeSeams(t)
	stubOrgSnapshot(t)

	// designated-only 流程不需要组织架构(把 provider 置空验证)
	rechargeActiveOrgProvider = func() string { return "" }

	chain, err := ResolveApprovalChain(100, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDesignated, UserIds: []int{301, 301}},
	})
	require.NoError(t, err)
	require.Len(t, chain, 1)
	require.Len(t, chain[0], 1) // 重复的 301 去重
	assert.Equal(t, 301, chain[0][0].UserId)
	assert.Equal(t, "指定人", chain[0][0].Name)
	assert.Equal(t, system_setting.RechargeLevelTypeDesignated, chain[0][0].LevelType)
}

func TestResolveApprovalChainDesignatedRejectsUnknownUser(t *testing.T) {
	resetRechargeSeams(t)
	stubOrgSnapshot(t)

	_, err := ResolveApprovalChain(100, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDesignated, UserIds: []int{999}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "指定的审批人不存在")
}

func TestResolveApprovalChainRequiresOrgForDeptLeader(t *testing.T) {
	resetRechargeSeams(t)
	rechargeActiveOrgProvider = func() string { return "" }

	_, err := ResolveApprovalChain(100, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDeptLeader, DeptLevel: 1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "组织架构同步未启用")
}

func TestResolveApprovalChainApplicantNotInSnapshot(t *testing.T) {
	resetRechargeSeams(t)
	stubOrgSnapshot(t)
	rechargeGetUserById = func(id int) (*model.User, error) {
		return &model.User{Id: id, Username: "ghost", DingTalkId: "u-ghost"}, nil
	}

	_, err := ResolveApprovalChain(500, []system_setting.RechargeApprovalLevel{
		{Type: system_setting.RechargeLevelTypeDeptLeader, DeptLevel: 1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不在组织架构快照中")
}

func TestWalkDeptUpCycleGuard(t *testing.T) {
	// 快照数据成环:2 -> 3 -> 2,上溯不能死循环
	deptByDeptId := map[string]model.OrgDepartment{
		"2": {DeptId: "2", ParentId: "3"},
		"3": {DeptId: "3", ParentId: "2"},
	}
	assert.Equal(t, "", walkDeptUp(deptByDeptId, "2", 10))
	assert.Equal(t, "3", walkDeptUp(deptByDeptId, "2", 1))
}

func TestRechargeQuotaFromUsd(t *testing.T) {
	quota, err := RechargeQuotaFromUsd(10)
	require.NoError(t, err)
	assert.Equal(t, int(10*common.QuotaPerUnit), quota)

	_, err = RechargeQuotaFromUsd(0)
	require.Error(t, err)
}
