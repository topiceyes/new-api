package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 组织通讯录接口的请求间隔,沿用离职巡检的限流口径。
const orgSyncRequestGap = 200 * time.Millisecond

// OrgSyncResult 一次组织架构同步的结果摘要,同时作为 SystemTask 的 result。
type OrgSyncResult struct {
	Provider      string `json:"provider"`
	Departments   int    `json:"departments"`
	Members       int    `json:"members"`
	Matched       int    `json:"matched"`        // 匹配到本地账号的成员数
	GroupMapped   int    `json:"group_mapped"`   // 本次被映射进目标分组的用户数
	GroupUnmapped int    `json:"group_unmapped"` // 本次卸任恢复原分组的用户数
}

var (
	// ErrOrgSyncNoProvider 钉钉/飞书互斥,两边都没启用时无法同步。
	ErrOrgSyncNoProvider = errors.New("no enterprise provider (dingtalk/feishu) is enabled")
	ErrOrgSyncNotConfigured = errors.New("enterprise app credentials are not configured")
)

// ActiveOrgSyncProvider 返回当前启用的企业 IM(钉钉/飞书互斥,只会命中一方)。
func ActiveOrgSyncProvider() string {
	if system_setting.GetDingTalkSettings().Enabled {
		return model.OrgProviderDingTalk
	}
	if system_setting.GetFeishuSettings().Enabled {
		return model.OrgProviderFeishu
	}
	return ""
}

// OrgSyncScheduleEnabled 调度器用:对应 provider 的同步开关与凭证都就绪才排期。
func OrgSyncScheduleEnabled() bool {
	switch ActiveOrgSyncProvider() {
	case model.OrgProviderDingTalk:
		s := system_setting.GetDingTalkSettings()
		return s.OrgSyncEnabled && s.AppKey != "" && s.AppSecret != ""
	case model.OrgProviderFeishu:
		s := system_setting.GetFeishuSettings()
		return s.OrgSyncEnabled && s.AppId != "" && s.AppSecret != ""
	}
	return false
}

// OrgSyncInterval 调度器用:同步间隔,非法值回落到 6 小时。
func OrgSyncInterval() time.Duration {
	hours := 0
	switch ActiveOrgSyncProvider() {
	case model.OrgProviderDingTalk:
		hours = system_setting.GetDingTalkSettings().OrgSyncIntervalHours
	case model.OrgProviderFeishu:
		hours = system_setting.GetFeishuSettings().OrgSyncIntervalHours
	}
	if hours < 1 || hours > 168 {
		hours = 6
	}
	return time.Duration(hours) * time.Hour
}

// RunOrgSyncOnce 执行一次全量同步:拉取 -> 匹配 -> 快照替换 + 分组映射,
// 全部在一个事务里落库。拉取阶段任何 API 失败都会中止,旧快照原样保留,
// 绝不用半成品覆盖(否则一次网络抖动就会让整棵组织树消失)。
func RunOrgSyncOnce(ctx context.Context) (*OrgSyncResult, error) {
	provider := ActiveOrgSyncProvider()
	if provider == "" {
		return nil, ErrOrgSyncNoProvider
	}

	var depts []model.OrgDepartment
	var members []model.OrgMember
	var err error
	switch provider {
	case model.OrgProviderDingTalk:
		settings := system_setting.GetDingTalkSettings()
		if settings.AppKey == "" || settings.AppSecret == "" {
			return nil, ErrOrgSyncNotConfigured
		}
		dingTalkProvider, ok := oauth.GetProvider("dingtalk").(*oauth.DingTalkProvider)
		if !ok {
			return nil, fmt.Errorf("dingtalk oauth provider not registered")
		}
		depts, members, err = dingTalkProvider.FetchDingTalkOrgSnapshot(ctx, orgSyncRequestGap)
	case model.OrgProviderFeishu:
		settings := system_setting.GetFeishuSettings()
		if settings.AppId == "" || settings.AppSecret == "" {
			return nil, ErrOrgSyncNotConfigured
		}
		feishuProvider, ok := oauth.GetProvider("feishu").(*oauth.FeishuProvider)
		if !ok {
			return nil, fmt.Errorf("feishu oauth provider not registered")
		}
		depts, members, err = feishuProvider.FetchFeishuOrgSnapshot(ctx, orgSyncRequestGap)
	}
	if err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	for i := range depts {
		depts[i].SyncedAt = now
	}
	for i := range members {
		members[i].SyncedAt = now
	}

	// 按 unionId 批量匹配本地账号(登录时已写入 dingtalk_id/feishu_id)。
	unionIds := make([]string, 0, len(members))
	for _, m := range members {
		unionIds = append(unionIds, m.UnionId)
	}
	userIdByUnion, err := model.GetUserIdsByOrgUnionIds(provider, unionIds)
	if err != nil {
		return nil, fmt.Errorf("match org members to users: %w", err)
	}
	result := &OrgSyncResult{Provider: provider, Departments: len(depts), Members: len(members)}
	for i := range members {
		if userId, ok := userIdByUnion[members[i].UnionId]; ok {
			members[i].UserId = userId
			result.Matched++
		}
	}

	// 主管分组映射(可选,默认关)。状态以旧快照的 GroupMapped/PrevGroup 为准,
	// 只动同步自己写入的分组:映射时记录原分组,卸任时仅当当前分组仍是目标组
	// 才恢复,期间管理员手动改过分组就不碰。
	mapGroup, targetGroup := orgSyncGroupMappingSettings(provider)
	var groupChanges []model.OrgGroupChange
	if mapGroup && targetGroup != "" {
		states, stateErr := model.GetOrgMemberGroupStates(provider)
		if stateErr != nil {
			return nil, fmt.Errorf("load org member group states: %w", stateErr)
		}
		inSnapshot := make(map[string]bool, len(members))
		userIds := make([]int, 0, len(members))
		for _, m := range members {
			inSnapshot[m.UnionId] = true
			if m.UserId > 0 {
				userIds = append(userIds, m.UserId)
			}
		}
		// 旧快照里已映射但这次消失的成员(离职/被移出企业)也要纳入卸任判定。
		for unionId, state := range states {
			if state.GroupMapped && state.UserId > 0 && !inSnapshot[unionId] {
				userIds = append(userIds, state.UserId)
			}
		}
		groups, groupErr := model.GetOrgUserGroups(userIds)
		if groupErr != nil {
			return nil, fmt.Errorf("load org user groups: %w", groupErr)
		}
		groupChanges = computeOrgGroupChanges(members, states, inSnapshot, groups, targetGroup, result)
	}

	appliedUserIds, err := model.ReplaceOrgSnapshot(provider, depts, members, groupChanges)
	if err != nil {
		return nil, fmt.Errorf("replace org snapshot: %w", err)
	}
	// 通讯录权威:用快照真实姓名回写已绑定用户的 display_name(覆盖注册时
	// 写入的 OAuth 昵称,也覆盖管理员手动修改),顺带完成存量用户回填。
	displayNameUpdated, err := model.SyncOrgMemberDisplayNames(members)
	if err != nil {
		// 回写失败不影响同步主结果,快照已生效;记日志等待下次同步重试。
		common.SysError(fmt.Sprintf("org sync: sync display names failed: provider=%s: %v", provider, err))
	}
	// 分组直写不走 EditWithTx(不升 auth_version、不踢会话),提交后刷新缓存。
	for _, userId := range appliedUserIds {
		if err := model.RefreshUserGroupCache(userId); err != nil {
			common.SysError(fmt.Sprintf("org sync: failed to refresh group cache for user %d: %v", userId, err))
		}
	}
	// 快照已替换,部门负责人范围缓存全部失效,下次请求按新快照重新推导。
	InvalidateAnalyticsScopeCache()
	common.SysLog(fmt.Sprintf("org sync finished: provider=%s, departments=%d, members=%d, matched=%d, group_mapped=%d, group_unmapped=%d, display_names=%d",
		result.Provider, result.Departments, result.Members, result.Matched, result.GroupMapped, result.GroupUnmapped, displayNameUpdated))
	return result, nil
}

func orgSyncGroupMappingSettings(provider string) (mapGroup bool, targetGroup string) {
	switch provider {
	case model.OrgProviderDingTalk:
		s := system_setting.GetDingTalkSettings()
		return s.OrgSyncMapGroup, s.OrgSyncTargetGroup
	case model.OrgProviderFeishu:
		s := system_setting.GetFeishuSettings()
		return s.OrgSyncMapGroup, s.OrgSyncTargetGroup
	}
	return false, ""
}

// computeOrgGroupChanges 对比新快照与旧映射状态,算出本次分组动作,并把
// GroupMapped/PrevGroup 写回新成员行(随快照一起落库)。纯函数:DB 读取
// 由调用方注入,便于表测试直接覆盖映射不变量。
func computeOrgGroupChanges(members []model.OrgMember, states map[string]model.OrgMember, inSnapshot map[string]bool, groups map[int]string, targetGroup string, result *OrgSyncResult) []model.OrgGroupChange {
	var changes []model.OrgGroupChange
	for i := range members {
		m := &members[i]
		if m.UserId <= 0 {
			continue
		}
		isLeader := orgMemberIsLeader(m.LeaderDeptIds)
		prev, wasMapped := states[m.UnionId]
		current := groups[m.UserId]
		switch {
		case isLeader && !wasMapped:
			// 新任主管:已在目标组则只标记,不写分组。
			m.GroupMapped = true
			m.PrevGroup = current
			if current != targetGroup {
				changes = append(changes, model.OrgGroupChange{UserId: m.UserId, ExpectCurrent: current, NewGroup: targetGroup})
				result.GroupMapped++
			}
		case isLeader && wasMapped:
			// 仍是主管:沿用旧映射状态。
			m.GroupMapped = true
			m.PrevGroup = prev.PrevGroup
		case !isLeader && wasMapped:
			// 卸任:当前分组仍是目标组才恢复到映射前分组。
			m.GroupMapped = false
			m.PrevGroup = ""
			if current == targetGroup {
				restore := prev.PrevGroup
				if restore == "" {
					restore = "default"
				}
				changes = append(changes, model.OrgGroupChange{UserId: m.UserId, ExpectCurrent: targetGroup, NewGroup: restore})
				result.GroupUnmapped++
			}
		}
	}
	// 消失在快照里的已映射成员按卸任处理(离职账号随后由离职巡检禁用,
	// 这里只是把分组恢复,保持状态不悬空)。
	for unionId, state := range states {
		if !state.GroupMapped || state.UserId <= 0 || inSnapshot[unionId] {
			continue
		}
		if groups[state.UserId] == targetGroup {
			restore := state.PrevGroup
			if restore == "" {
				restore = "default"
			}
			changes = append(changes, model.OrgGroupChange{UserId: state.UserId, ExpectCurrent: targetGroup, NewGroup: restore})
			result.GroupUnmapped++
		}
	}
	return changes
}

func orgMemberIsLeader(leaderDeptIdsJson string) bool {
	var deptIds []string
	if err := common.UnmarshalJsonStr(leaderDeptIdsJson, &deptIds); err != nil {
		return false
	}
	return len(deptIds) > 0
}
