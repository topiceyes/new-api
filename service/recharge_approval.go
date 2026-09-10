package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 充值申请审批链解析:提交申请时按流程配置把每级审批人解析为具体用户并快照。
// dept_leader 级 = 申请人的第 N 级部门主管(1=所在部门,2=上级部门,...),
// 部门主管取自 org 快照成员的 LeaderDeptIds(OrgDepartment.LeaderUserIds
// 只有飞书抓取器填充,不能用);designated 级 = 配置的指定用户。
// 规则:排除申请人本人(不能自审)、跳过未绑定本地账号的主管;
// 目标部门没有可用主管时沿部门链继续向上递归,直到有主管为止;
// 整条链都没有可用主管时兜底到 root 管理员(rechargeRootFallbackUserId);
// 兜底也不可用时该级解析失败,提交被阻断(宁缺毋滥)。

// 查找函数做成包级 seam,便于无数据库的单测(仿 admin_alert.go)。
var (
	rechargeActiveOrgProvider    = ActiveOrgSyncProvider
	rechargeGetUserById          = func(id int) (*model.User, error) { return model.GetUserById(id, false) }
	rechargeGetOrgMemberByUnion  = model.GetOrgMemberByUnionId
	rechargeGetOrgDepartments    = model.GetOrgDepartments
	rechargeGetOrgMembers        = model.GetOrgMembers
	rechargeGetUserIdsByUnionIds = model.GetUserIdsByOrgUnionIds
)

// rechargeMaxDeptDepth 部门链上溯的安全上限,防快照数据成环时死循环。
const rechargeMaxDeptDepth = 16

// rechargeRootFallbackUserId 部门主管链完全解析不到人时的兜底审批人:root 管理员。
const rechargeRootFallbackUserId = 1

// orgUnionIdColumn 返回该 provider 下 users 表存 unionId 的字段值。
func orgUnionIdColumn(provider string, user *model.User) string {
	if provider == model.OrgProviderFeishu {
		return user.FeishuId
	}
	return user.DingTalkId
}

// deptLeaderUnionIds 扫描成员快照,返回 deptId 的主管 unionId 列表。
func deptLeaderUnionIds(members []model.OrgMember, deptId string) []string {
	var unionIds []string
	for i := range members {
		var leaderDeptIds []string
		if err := common.UnmarshalJsonStr(members[i].LeaderDeptIds, &leaderDeptIds); err != nil {
			continue
		}
		for _, id := range leaderDeptIds {
			if id == deptId {
				unionIds = append(unionIds, members[i].UnionId)
				break
			}
		}
	}
	return unionIds
}

// walkDeptUpAtMost 从 deptId 沿 ParentId 上溯至多 steps 层,链不够长时停在最顶层部门。
// 起点部门不在快照或部门链成环时返回空串。
func walkDeptUpAtMost(deptByDeptId map[string]model.OrgDepartment, deptId string, steps int) string {
	if _, ok := deptByDeptId[deptId]; !ok {
		return ""
	}
	visited := map[string]struct{}{deptId: {}}
	current := deptId
	for i := 0; i < steps; i++ {
		parent := deptByDeptId[current].ParentId
		if parent == "" {
			break // 已到最顶层
		}
		if _, ok := visited[parent]; ok {
			return "" // 成环,快照数据异常
		}
		if _, ok := deptByDeptId[parent]; !ok {
			break // 父部门不在快照,停在当前部门
		}
		visited[parent] = struct{}{}
		current = parent
	}
	return current
}

// resolveDeptLeaderLevel 解析一个 dept_leader 级:主部门上溯 k-1 层后的主管们。
// 目标层级超出部门链长度时退到最顶层部门;目标部门没有可用主管(未配置、
// 全是申请人本人或未绑定账号)时沿 ParentId 继续向上递归,直到有主管为止;
// 整条链都没有可用主管时兜底到 root 管理员。
func resolveDeptLeaderLevel(provider string, applicantUserId int, applicantUnionId string, primaryDeptId string,
	deptLevel int, deptByDeptId map[string]model.OrgDepartment, members []model.OrgMember,
	memberByUnionId map[string]model.OrgMember) ([]model.ResolvedRechargeApprover, error) {

	targetDeptId := walkDeptUpAtMost(deptByDeptId, primaryDeptId, deptLevel-1)
	if targetDeptId == "" {
		return nil, errors.New("申请人的部门不在组织架构快照中或部门数据成环,请联系管理员")
	}

	visited := map[string]struct{}{}
	current := targetDeptId
	for depth := 0; depth < rechargeMaxDeptDepth; depth++ {
		if _, ok := visited[current]; ok {
			break
		}
		visited[current] = struct{}{}
		unionIds := deptLeaderUnionIds(members, current)
		if len(unionIds) > 0 {
			userIdByUnionId, err := rechargeGetUserIdsByUnionIds(provider, unionIds)
			if err != nil {
				return nil, err
			}
			approvers := make([]model.ResolvedRechargeApprover, 0, len(unionIds))
			for _, unionId := range unionIds {
				if unionId == applicantUnionId {
					continue // 不能自审
				}
				userId := userIdByUnionId[unionId]
				if userId <= 0 {
					continue // 主管未绑定本地账号,无法登录审批
				}
				name := unionId
				if m, ok := memberByUnionId[unionId]; ok && m.Name != "" {
					name = m.Name
				}
				approvers = append(approvers, model.ResolvedRechargeApprover{
					UserId:    userId,
					UnionId:   unionId,
					Name:      name,
					LevelType: system_setting.RechargeLevelTypeDeptLeader,
				})
			}
			if len(approvers) > 0 {
				return approvers, nil
			}
		}
		dept, ok := deptByDeptId[current]
		if !ok || dept.ParentId == "" {
			break
		}
		current = dept.ParentId
	}

	// 整条部门链都没有可用主管,兜底到 root 管理员。
	if applicantUserId == rechargeRootFallbackUserId {
		return nil, errors.New("部门链上没有配置主管,且兜底审批人(root 管理员)不能审批自己的申请,请联系管理员调整审批流程")
	}
	root, err := rechargeGetUserById(rechargeRootFallbackUserId)
	if err != nil || root == nil || root.Id == 0 {
		return nil, errors.New("部门链上没有配置主管,且兜底审批人(root 管理员)不存在,请联系管理员调整审批流程")
	}
	name := root.Username
	if root.DisplayName != "" {
		name = root.DisplayName
	}
	return []model.ResolvedRechargeApprover{{
		UserId:    root.Id,
		UnionId:   "", // 通知按 userId 兜底查绑定,与 designated 一致
		Name:      name,
		LevelType: system_setting.RechargeLevelTypeDeptLeader,
	}}, nil
}

// resolveDesignatedLevel 解析一个 designated 级:配置的指定用户。
func resolveDesignatedLevel(applicantUserId int, userIds []int) ([]model.ResolvedRechargeApprover, error) {
	approvers := make([]model.ResolvedRechargeApprover, 0, len(userIds))
	seen := map[int]struct{}{}
	for _, userId := range userIds {
		if _, ok := seen[userId]; ok {
			continue
		}
		seen[userId] = struct{}{}
		if userId == applicantUserId {
			continue // 不能自审
		}
		user, err := rechargeGetUserById(userId)
		if err != nil || user == nil || user.Id == 0 {
			return nil, fmt.Errorf("指定的审批人不存在: user_id=%d", userId)
		}
		name := user.Username
		if user.DisplayName != "" {
			name = user.DisplayName
		}
		approvers = append(approvers, model.ResolvedRechargeApprover{
			UserId:    userId,
			UnionId:   "", // designated 审批人未必绑定 IM,通知时按 userId 兜底查
			Name:      name,
			LevelType: system_setting.RechargeLevelTypeDesignated,
		})
	}
	if len(approvers) == 0 {
		return nil, errors.New("指定审批人级别过滤后无人可审(不能指定申请人本人)")
	}
	return approvers, nil
}

// ResolveApprovalChain 按流程配置解析申请人的完整审批链。
// 返回 [][]ResolvedRechargeApprover,外层下标 = 级别-1。
func ResolveApprovalChain(applicantUserId int, levels []system_setting.RechargeApprovalLevel) ([][]model.ResolvedRechargeApprover, error) {
	if len(levels) == 0 {
		return nil, errors.New("审批流程未配置审批级别")
	}

	needsOrg := false
	for _, level := range levels {
		if level.Type == system_setting.RechargeLevelTypeDeptLeader {
			needsOrg = true
			break
		}
	}

	var (
		provider         string
		applicantUnionId string
		primaryDeptId    string
		deptByDeptId     map[string]model.OrgDepartment
		members          []model.OrgMember
		memberByUnionId  map[string]model.OrgMember
	)
	if needsOrg {
		provider = rechargeActiveOrgProvider()
		if provider == "" {
			return nil, errors.New("流程包含部门主管级别,但组织架构同步未启用,请联系管理员")
		}
		user, err := rechargeGetUserById(applicantUserId)
		if err != nil || user == nil || user.Id == 0 {
			return nil, errors.New("申请人不存在")
		}
		applicantUnionId = orgUnionIdColumn(provider, user)
		member, err := rechargeGetOrgMemberByUnion(provider, applicantUnionId)
		if err != nil {
			return nil, err
		}
		if member == nil {
			return nil, errors.New("你不在组织架构快照中,无法解析部门主管,请联系管理员")
		}
		var deptIds []string
		if err := common.UnmarshalJsonStr(member.DeptIds, &deptIds); err != nil || len(deptIds) == 0 {
			return nil, errors.New("你在组织架构中没有部门信息,请联系管理员")
		}
		primaryDeptId = deptIds[0] // 与 usage_analytics 归因一致:取第一个部门

		depts, err := rechargeGetOrgDepartments(provider)
		if err != nil {
			return nil, err
		}
		deptByDeptId = make(map[string]model.OrgDepartment, len(depts))
		for _, d := range depts {
			deptByDeptId[d.DeptId] = d
		}
		members, err = rechargeGetOrgMembers(provider)
		if err != nil {
			return nil, err
		}
		memberByUnionId = make(map[string]model.OrgMember, len(members))
		for _, m := range members {
			memberByUnionId[m.UnionId] = m
		}
	}

	chain := make([][]model.ResolvedRechargeApprover, 0, len(levels))
	for i, level := range levels {
		var approvers []model.ResolvedRechargeApprover
		var err error
		switch level.Type {
		case system_setting.RechargeLevelTypeDeptLeader:
			approvers, err = resolveDeptLeaderLevel(provider, applicantUserId, applicantUnionId, primaryDeptId,
				level.DeptLevel, deptByDeptId, members, memberByUnionId)
		case system_setting.RechargeLevelTypeDesignated:
			approvers, err = resolveDesignatedLevel(applicantUserId, level.UserIds)
		default:
			err = fmt.Errorf("未知的审批人类型: %s", level.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("第 %d 级审批人解析失败: %s", i+1, err.Error())
		}
		chain = append(chain, approvers)
	}
	return chain, nil
}

// RechargeQuotaFromUsd 流程固定额度(美元)按当前 QuotaPerUnit 换算 quota。
func RechargeQuotaFromUsd(quotaUsd float64) (int, error) {
	quota := common.QuotaFromFloat(quotaUsd * common.QuotaPerUnit)
	if quota <= 0 {
		return 0, errors.New("充值额度换算结果无效")
	}
	return quota, nil
}
