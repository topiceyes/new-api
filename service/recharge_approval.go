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
// 任一级解析结果为空即整个解析失败,提交被阻断(宁缺毋滥)。

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

// walkDeptUp 从 deptId 沿 ParentId 上溯 steps 层,返回目标部门;链不够长返回空串。
func walkDeptUp(deptByDeptId map[string]model.OrgDepartment, deptId string, steps int) string {
	visited := map[string]struct{}{}
	current := deptId
	for i := 0; i < steps; i++ {
		if _, ok := visited[current]; ok {
			return ""
		}
		visited[current] = struct{}{}
		dept, ok := deptByDeptId[current]
		if !ok || dept.ParentId == "" {
			return ""
		}
		current = dept.ParentId
	}
	return current
}

// resolveDeptLeaderLevel 解析一个 dept_leader 级:主部门上溯 k-1 层后的主管们。
func resolveDeptLeaderLevel(provider string, applicantUnionId string, primaryDeptId string,
	deptLevel int, deptByDeptId map[string]model.OrgDepartment, members []model.OrgMember,
	memberByUnionId map[string]model.OrgMember) ([]model.ResolvedRechargeApprover, error) {

	targetDeptId := walkDeptUp(deptByDeptId, primaryDeptId, deptLevel-1)
	if targetDeptId == "" {
		return nil, fmt.Errorf("组织架构中没有第 %d 级部门(部门链不够长)", deptLevel)
	}
	targetDept := deptByDeptId[targetDeptId]
	unionIds := deptLeaderUnionIds(members, targetDeptId)
	if len(unionIds) == 0 {
		return nil, fmt.Errorf("部门「%s」没有配置主管", targetDept.Name)
	}
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
	if len(approvers) == 0 {
		return nil, fmt.Errorf("部门「%s」的主管都无法审批(未绑定账号或为申请人本人)", targetDept.Name)
	}
	return approvers, nil
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
			approvers, err = resolveDeptLeaderLevel(provider, applicantUnionId, primaryDeptId,
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
