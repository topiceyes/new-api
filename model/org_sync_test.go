package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedOrgSnapshot(t *testing.T, provider string) {
	t.Helper()
	require.NoError(t, DB.Create(&OrgDepartment{
		Provider: provider, DeptId: "1", ParentId: "", Name: "全体成员", MemberCount: 2,
	}).Error)
	require.NoError(t, DB.Create(&OrgDepartment{
		Provider: provider, DeptId: "2", ParentId: "1", Name: "研发部", MemberCount: 1,
	}).Error)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: provider, UnionId: "u-a", ProviderUserId: "pa", Name: "甲",
		DeptIds: `["1","2"]`, LeaderDeptIds: `["2"]`,
	}).Error)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: provider, UnionId: "u-b", ProviderUserId: "pb", Name: "乙",
		DeptIds: `["1"]`, LeaderDeptIds: `[]`,
	}).Error)
}

// 快照替换必须整体生效:旧行消失、新行落库、其他 provider 的数据不受影响。
func TestReplaceOrgSnapshotSwapsProviderDataOnly(t *testing.T) {
	truncateTables(t)

	seedOrgSnapshot(t, OrgProviderDingTalk)
	seedOrgSnapshot(t, OrgProviderFeishu)

	newDepts := []OrgDepartment{
		{Provider: OrgProviderDingTalk, DeptId: "1", ParentId: "", Name: "全体成员", MemberCount: 1},
		{Provider: OrgProviderDingTalk, DeptId: "9", ParentId: "1", Name: "新部门", MemberCount: 1},
	}
	newMembers := []OrgMember{
		{Provider: OrgProviderDingTalk, UnionId: "u-c", ProviderUserId: "pc", Name: "丙",
			DeptIds: `["1","9"]`, LeaderDeptIds: `[]`},
	}
	applied, err := ReplaceOrgSnapshot(OrgProviderDingTalk, newDepts, newMembers, nil)
	require.NoError(t, err)
	assert.Empty(t, applied)

	depts, err := GetOrgDepartments(OrgProviderDingTalk)
	require.NoError(t, err)
	require.Len(t, depts, 2)
	assert.Equal(t, "新部门", depts[1].Name)

	members, err := GetOrgMembers(OrgProviderDingTalk)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "u-c", members[0].UnionId)

	// 飞书侧快照原样保留。
	feishuDepts, err := GetOrgDepartments(OrgProviderFeishu)
	require.NoError(t, err)
	assert.Len(t, feishuDepts, 2)
}

// 分组变更带 ExpectCurrent 乐观守卫:当前分组不等于期望时必须跳过,
// 防止覆盖同步间隙管理员的手动调整;并返回实际生效的用户列表供缓存刷新。
func TestReplaceOrgSnapshotGroupChangeGuard(t *testing.T) {
	truncateTables(t)

	user1 := &User{Username: "guarded", Group: "default", AffCode: "guarded"}
	require.NoError(t, DB.Create(user1).Error)
	user2 := &User{Username: "drifted", Group: "vip", AffCode: "drifted"} // 已被管理员手动调到 vip
	require.NoError(t, DB.Create(user2).Error)

	changes := []OrgGroupChange{
		{UserId: user1.Id, ExpectCurrent: "default", NewGroup: "vip"},
		{UserId: user2.Id, ExpectCurrent: "default", NewGroup: "svip"}, // 期望与实际不符
	}
	applied, err := ReplaceOrgSnapshot(OrgProviderDingTalk, nil, nil, changes)
	require.NoError(t, err)
	assert.Equal(t, []int{user1.Id}, applied)

	groups, err := GetOrgUserGroups([]int{user1.Id, user2.Id})
	require.NoError(t, err)
	assert.Equal(t, "vip", groups[user1.Id])
	assert.Equal(t, "vip", groups[user2.Id]) // 未被改成 svip
}

// unionId 匹配按 provider 走对应绑定列,未知 provider 必须报错而不是静默空匹配。
func TestGetUserIdsByOrgUnionIdsMatchesBindingColumn(t *testing.T) {
	truncateTables(t)

	dingUser := &User{Username: "ding", DingTalkId: "union-ding", AffCode: "ding"}
	require.NoError(t, DB.Create(dingUser).Error)
	feishuUser := &User{Username: "fei", FeishuId: "union-fei", AffCode: "fei"}
	require.NoError(t, DB.Create(feishuUser).Error)

	matched, err := GetUserIdsByOrgUnionIds(OrgProviderDingTalk, []string{"union-ding", "union-fei", "ghost"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"union-ding": dingUser.Id}, matched)

	matched, err = GetUserIdsByOrgUnionIds(OrgProviderFeishu, []string{"union-ding", "union-fei"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"union-fei": feishuUser.Id}, matched)

	_, err = GetUserIdsByOrgUnionIds("wecom", []string{"x"})
	assert.ErrorIs(t, err, ErrOrgProviderUnsupported)
}

// 成员列表要按 UserId 补充本地账号名,未绑定的保持空值(前端「未绑定」徽标依据)。
func TestGetOrgMembersEnrichesBoundUser(t *testing.T) {
	truncateTables(t)

	bound := &User{Username: "bound_user", DisplayName: "绑定用户", AffCode: "bound_user"}
	require.NoError(t, DB.Create(bound).Error)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: OrgProviderDingTalk, UnionId: "u-bound", Name: "甲", UserId: bound.Id,
		DeptIds: `["1"]`, LeaderDeptIds: `[]`,
	}).Error)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: OrgProviderDingTalk, UnionId: "u-free", Name: "乙",
		DeptIds: `["1"]`, LeaderDeptIds: `[]`,
	}).Error)

	members, err := GetOrgMembers(OrgProviderDingTalk)
	require.NoError(t, err)
	require.Len(t, members, 2)
	assert.Equal(t, "bound_user", members[0].Username)
	assert.Equal(t, "绑定用户", members[0].DisplayName)
	assert.Equal(t, 0, members[1].UserId)
	assert.Empty(t, members[1].Username)
}

// 分组映射状态必须能跨快照轮换存活:同步引擎靠它区分「同步加的组」和
// 「管理员手动调的组」。
func TestGetOrgMemberGroupStatesRoundTrip(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&OrgMember{
		Provider: OrgProviderDingTalk, UnionId: "u-mapped", UserId: 7,
		GroupMapped: true, PrevGroup: "default",
		DeptIds: `["1"]`, LeaderDeptIds: `["1"]`,
	}).Error)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: OrgProviderDingTalk, UnionId: "u-plain",
		DeptIds: `["1"]`, LeaderDeptIds: `[]`,
	}).Error)

	states, err := GetOrgMemberGroupStates(OrgProviderDingTalk)
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.True(t, states["u-mapped"].GroupMapped)
	assert.Equal(t, "default", states["u-mapped"].PrevGroup)
	assert.False(t, states["u-plain"].GroupMapped)
}

// 登录即绑定:定点把快照行的 UserId 补齐;已绑定到同一用户的行不重写,
// 快照里还没有该成员时不算错误,其他 provider 的同 unionId 行不受影响。
func TestBindOrgMemberToUser(t *testing.T) {
	truncateTables(t)
	seedOrgSnapshot(t, OrgProviderDingTalk)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: OrgProviderFeishu, UnionId: "u-a", ProviderUserId: "fa", Name: "甲(飞书)",
		DeptIds: `["1"]`, LeaderDeptIds: `[]`,
	}).Error)

	require.NoError(t, BindOrgMemberToUser(OrgProviderDingTalk, "u-a", 42))

	members, err := GetOrgMembers(OrgProviderDingTalk)
	require.NoError(t, err)
	require.Len(t, members, 2)
	byUnion := map[string]OrgMember{}
	for _, m := range members {
		byUnion[m.UnionId] = m
	}
	assert.Equal(t, 42, byUnion["u-a"].UserId)
	assert.Equal(t, 0, byUnion["u-b"].UserId)

	// 其他 provider 的同名 unionId 不受影响。
	feishuMembers, err := GetOrgMembers(OrgProviderFeishu)
	require.NoError(t, err)
	require.Len(t, feishuMembers, 1)
	assert.Equal(t, 0, feishuMembers[0].UserId)

	// 重复绑定同一用户是幂等空操作;绑定到别的用户则更新(账号换绑场景)。
	require.NoError(t, BindOrgMemberToUser(OrgProviderDingTalk, "u-a", 42))
	require.NoError(t, BindOrgMemberToUser(OrgProviderDingTalk, "u-a", 43))
	rebound, err := GetOrgMembers(OrgProviderDingTalk)
	require.NoError(t, err)
	for _, m := range rebound {
		if m.UnionId == "u-a" {
			assert.Equal(t, 43, m.UserId)
		}
	}

	// 快照里不存在的成员、空 unionId、非法 userId 都是空操作。
	require.NoError(t, BindOrgMemberToUser(OrgProviderDingTalk, "u-unknown", 7))
	require.NoError(t, BindOrgMemberToUser(OrgProviderDingTalk, "", 7))
	require.NoError(t, BindOrgMemberToUser(OrgProviderDingTalk, "u-a", 0))
	assert.ErrorIs(t, BindOrgMemberToUser("github", "u-a", 7), ErrOrgProviderUnsupported)
}

// 快照真名查询:命中返回通讯录姓名;未收录、空姓名、空 unionId 均返回空串
// 不报错(登录路径据此回退昵称);非组织 provider 报错。
func TestGetOrgMemberRealName(t *testing.T) {
	truncateTables(t)
	seedOrgSnapshot(t, OrgProviderDingTalk)
	require.NoError(t, DB.Create(&OrgMember{
		Provider: OrgProviderDingTalk, UnionId: "u-noname", ProviderUserId: "pn",
		Name: "", DeptIds: `["1"]`, LeaderDeptIds: `[]`,
	}).Error)

	name, err := GetOrgMemberRealName(OrgProviderDingTalk, "u-a")
	require.NoError(t, err)
	assert.Equal(t, "甲", name)

	name, err = GetOrgMemberRealName(OrgProviderDingTalk, "u-unknown")
	require.NoError(t, err)
	assert.Empty(t, name)

	name, err = GetOrgMemberRealName(OrgProviderDingTalk, "u-noname")
	require.NoError(t, err)
	assert.Empty(t, name)

	name, err = GetOrgMemberRealName(OrgProviderDingTalk, "")
	require.NoError(t, err)
	assert.Empty(t, name)

	_, err = GetOrgMemberRealName("github", "u-a")
	assert.ErrorIs(t, err, ErrOrgProviderUnsupported)
}

// 同步回写 display_name:通讯录权威覆盖昵称与管理员手改;未绑定成员、空
// 姓名跳过;已一致的不重复写;超长截断到 20 字符(与 validate max=20 对齐)。
func TestSyncOrgMemberDisplayNames(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&User{Username: "dingtalk_1", DisplayName: "昵称甲", Role: 1, Status: 1, AffCode: "aff-sync-1"}).Error)
	require.NoError(t, DB.Create(&User{Username: "dingtalk_2", DisplayName: "乙", Role: 1, Status: 1, AffCode: "aff-sync-2"}).Error)
	require.NoError(t, DB.Create(&User{Username: "dingtalk_3", DisplayName: "管理员改的名", Role: 1, Status: 1, AffCode: "aff-sync-3"}).Error)
	var users []User
	require.NoError(t, DB.Select("id", "username").Order("id").Find(&users).Error)
	require.Len(t, users, 3)

	longName := "这是一个非常非常长的姓名超过了二十个字符限制需要截断"
	members := []OrgMember{
		{UnionId: "u-a", UserId: users[0].Id, Name: "甲"},        // 昵称 -> 真名
		{UnionId: "u-b", UserId: users[1].Id, Name: "乙"},        // 已一致,不写
		{UnionId: "u-c", UserId: users[2].Id, Name: "丙"},        // 覆盖管理员手改
		{UnionId: "u-d", UserId: 0, Name: "丁"},                  // 未绑定,跳过
		{UnionId: "u-e", UserId: users[0].Id, Name: ""},          // 空姓名,跳过(同 user 已有真名条目)
		{UnionId: "u-f", UserId: 99999, Name: "不存在"},          // user 不存在,不影响
		{UnionId: "u-g", UserId: users[1].Id, Name: longName},    // 超长截断
	}

	updated, err := SyncOrgMemberDisplayNames(members)
	require.NoError(t, err)
	assert.Equal(t, 3, updated)

	var got []User
	require.NoError(t, DB.Select("id", "display_name").Order("id").Find(&got).Error)
	assert.Equal(t, "甲", got[0].DisplayName)
	assert.Equal(t, longName[:60], got[1].DisplayName) // 20 个汉字 = 60 字节
	assert.Equal(t, "丙", got[2].DisplayName)

	// 空输入幂等。
	updated, err = SyncOrgMemberDisplayNames(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, updated)
}

// 登录路径直写刷新:正常更新;空名/非法 id 是幂等空操作。
func TestUpdateUserDisplayNameDirect(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "dingtalk_1", DisplayName: "昵称", Role: 1, Status: 1}).Error)
	var user User
	require.NoError(t, DB.Select("id").Where("username = ?", "dingtalk_1").Take(&user).Error)

	require.NoError(t, UpdateUserDisplayNameDirect(user.Id, "真实姓名"))
	var reloaded User
	require.NoError(t, DB.Select("display_name").Where("id = ?", user.Id).Take(&reloaded).Error)
	assert.Equal(t, "真实姓名", reloaded.DisplayName)

	require.NoError(t, UpdateUserDisplayNameDirect(user.Id, ""))
	require.NoError(t, UpdateUserDisplayNameDirect(0, "x"))
	reloaded = User{}
	require.NoError(t, DB.Select("display_name").Where("id = ?", user.Id).Take(&reloaded).Error)
	assert.Equal(t, "真实姓名", reloaded.DisplayName)
}

// 批量补充:按 user_id / username 两种键都能取到真名,未知键静默缺省。
func TestGetUserDisplayNames(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "dingtalk_1", DisplayName: "甲", Role: 1, Status: 1, AffCode: "aff-names-1"}).Error)
	require.NoError(t, DB.Create(&User{Username: "dingtalk_2", DisplayName: "", Role: 1, Status: 1, AffCode: "aff-names-2"}).Error)
	var users []User
	require.NoError(t, DB.Select("id", "username").Order("id").Find(&users).Error)
	require.Len(t, users, 2)

	byId, err := GetUserDisplayNames([]int{users[0].Id, users[1].Id, 99999})
	require.NoError(t, err)
	assert.Equal(t, "甲", byId[users[0].Id])
	assert.Equal(t, "", byId[users[1].Id])
	_, ok := byId[99999]
	assert.False(t, ok)

	byName, err := GetUserDisplayNamesByUsernames([]string{"dingtalk_1", "ghost"})
	require.NoError(t, err)
	assert.Equal(t, "甲", byName["dingtalk_1"])
	_, ok = byName["ghost"]
	assert.False(t, ok)

	empty, err := GetUserDisplayNames(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// 日志列表 enrich:按 user_id 补真名;无匹配用户的行留空不报错。
func TestGetAllLogsFillsDisplayName(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Username: "dingtalk_1", DisplayName: "甲", Role: 1, Status: 1}).Error)
	var user User
	require.NoError(t, DB.Select("id").Where("username = ?", "dingtalk_1").Take(&user).Error)

	require.NoError(t, createLog(&Log{UserId: user.Id, Username: "dingtalk_1", Type: LogTypeConsume, ModelName: "gpt-4o", CreatedAt: 1000}))
	require.NoError(t, createLog(&Log{UserId: 99999, Username: "ghost", Type: LogTypeConsume, ModelName: "gpt-4o", CreatedAt: 1001}))

	logs, total, err := GetAllLogs(LogTypeConsume, 0, 0, "", nil, "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, logs, 2)
	byUser := map[int]*Log{}
	for _, l := range logs {
		byUser[l.UserId] = l
	}
	assert.Equal(t, "甲", byUser[user.Id].DisplayName)
	assert.Empty(t, byUser[99999].DisplayName)
}
