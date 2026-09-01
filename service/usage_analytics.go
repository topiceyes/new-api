package service

import (
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// AnalyticsScope 使用分析的可见范围。UserIds 为 nil 表示不限制(管理员);
// 非 nil 为部门负责人的子树成员 user_id 集合。
type AnalyticsScope struct {
	Scope       string            `json:"scope"` // "admin" | "dept"
	UserIds     []int             `json:"-"`
	DeptIds     []string          `json:"dept_ids"`
	DeptNames   map[string]string `json:"-"`
	PrimaryDept map[int]string    `json:"-"` // userId -> 主部门(成员 DeptIds[0]),防重复计数
}

var ErrAnalyticsForbidden = errors.New("no usage analytics permission")

type analyticsScopeCacheEntry struct {
	scope     *AnalyticsScope
	expiresAt time.Time
}

var analyticsScopeCache sync.Map // userId -> analyticsScopeCacheEntry

const analyticsScopeCacheTTL = 120 * time.Second

// ResolveAnalyticsScope 推导用户的使用分析可见范围:
//  1. 管理员(role>=10) → 全量,不查 org 表;
//  2. 部门负责人 → org 快照主管字段推导: unionId → LeaderDeptIds → 部门树向下展开
//     → 子树内已绑定成员的 user_id 集合。快照同步后最多 120s(缓存 TTL)内生效。
//  3. 其他 → ErrAnalyticsForbidden。
func ResolveAnalyticsScope(userId int, role int) (*AnalyticsScope, error) {
	if role >= common.RoleAdminUser {
		return &AnalyticsScope{Scope: "admin"}, nil
	}
	if cached, ok := analyticsScopeCache.Load(userId); ok {
		entry := cached.(analyticsScopeCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			if entry.scope == nil {
				return nil, ErrAnalyticsForbidden
			}
			return entry.scope, nil
		}
		analyticsScopeCache.Delete(userId)
	}
	scope, err := resolveDeptLeaderScope(userId)
	if err != nil {
		// 无权结论也缓存(DB 错误不缓存),否则每个普通员工每次开 dashboard
		// 都要打两次库(GetUserById + GetOrgMemberByUnionId)。
		if errors.Is(err, ErrAnalyticsForbidden) {
			analyticsScopeCache.Store(userId, analyticsScopeCacheEntry{
				expiresAt: time.Now().Add(analyticsScopeCacheTTL),
			})
		}
		return nil, err
	}
	analyticsScopeCache.Store(userId, analyticsScopeCacheEntry{
		scope:     scope,
		expiresAt: time.Now().Add(analyticsScopeCacheTTL),
	})
	return scope, nil
}

// InvalidateAnalyticsScopeCache 组织快照替换后调用,避免负责人范围沿用旧快照。
// 用 Clear 而不是整体替换成新 sync.Map——后者与并发 Load/Store 存在数据竞争。
func InvalidateAnalyticsScopeCache() {
	analyticsScopeCache.Clear()
}

func resolveDeptLeaderScope(userId int) (*AnalyticsScope, error) {
	provider := ActiveOrgSyncProvider()
	if provider == "" {
		return nil, ErrAnalyticsForbidden
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, err
	}
	unionId := user.DingTalkId
	if provider == model.OrgProviderFeishu {
		unionId = user.FeishuId
	}
	// unionId 来自 users 表的 OAuth 绑定,本身就是身份权威,查到的成员即本人,
	// 无需再校验 member.UserId == userId(快照尚未回写 UserId 时也必须可用)。
	member, err := model.GetOrgMemberByUnionId(provider, unionId)
	if err != nil {
		return nil, err
	}
	if member == nil || member.LeaderDeptIds == "" {
		return nil, ErrAnalyticsForbidden
	}
	var leaderDeptIds []string
	if err := common.UnmarshalJsonStr(member.LeaderDeptIds, &leaderDeptIds); err != nil || len(leaderDeptIds) == 0 {
		return nil, ErrAnalyticsForbidden
	}

	attribution, err := LoadOrgDeptAttribution(provider)
	if err != nil {
		return nil, err
	}

	// 从负责人部门向下 BFS 展开整棵子树。
	subtree := make(map[string]bool)
	queue := append([]string{}, leaderDeptIds...)
	for len(queue) > 0 {
		deptId := queue[0]
		queue = queue[1:]
		if subtree[deptId] {
			continue
		}
		subtree[deptId] = true
		queue = append(queue, attribution.children[deptId]...)
	}

	scope := &AnalyticsScope{
		Scope:       "dept",
		DeptIds:     make([]string, 0, len(subtree)),
		DeptNames:   make(map[string]string, len(subtree)),
		PrimaryDept: make(map[int]string),
		UserIds:     []int{userId}, // 负责人总是包含自己
	}
	for deptId := range subtree {
		scope.DeptIds = append(scope.DeptIds, deptId)
		if name, ok := attribution.DeptNames[deptId]; ok {
			scope.DeptNames[deptId] = name
		}
	}
	seen := map[int]bool{userId: true}
	for _, m := range attribution.members {
		if m.UserId <= 0 || seen[m.UserId] {
			continue
		}
		for _, deptId := range m.deptIds {
			if subtree[deptId] {
				scope.UserIds = append(scope.UserIds, m.UserId)
				// 主部门取该成员在负责人子树内的第一个部门,保证部门卡片的
				// 归因一定落在可见部门里(DeptIds[0] 可能在子树外)。
				scope.PrimaryDept[m.UserId] = deptId
				seen[m.UserId] = true
				break
			}
		}
	}
	// 负责人自己的主部门同样优先取子树内的部门,否则部门卡片里看不到自己。
	for _, deptId := range memberDeptIds(member) {
		if subtree[deptId] {
			scope.PrimaryDept[userId] = deptId
			break
		}
	}
	return scope, nil
}

func memberDeptIds(m *model.OrgMember) []string {
	var deptIds []string
	if err := common.UnmarshalJsonStr(m.DeptIds, &deptIds); err != nil {
		return nil
	}
	return deptIds
}

// OrgDeptAttribution 组织快照的部门归因数据:部门树、名称、成员主部门。
// 管理员的部门对比卡片和负责人范围展开共用。
type OrgDeptAttribution struct {
	DeptNames    map[string]string // deptId -> 名称
	MemberCount  map[string]int    // deptId -> 已绑定成员数(按主部门计)
	PrimaryDept  map[int]string    // userId -> 主部门
	BoundUserIds []int             // 所有已绑定成员

	children map[string][]string // parentDeptId -> 子部门,内部展开用
	members  []orgMemberLite
}

type orgMemberLite struct {
	UserId  int
	deptIds []string
}

// LoadOrgDeptAttribution 加载当前快照并构建归因结构。多部门成员按 DeptIds[0]
// 归主部门,保证部门汇总不重复计数。
func LoadOrgDeptAttribution(provider string) (*OrgDeptAttribution, error) {
	depts, err := model.GetOrgDepartments(provider)
	if err != nil {
		return nil, err
	}
	members, err := model.GetOrgMembers(provider)
	if err != nil {
		return nil, err
	}
	attribution := &OrgDeptAttribution{
		DeptNames:   make(map[string]string, len(depts)),
		MemberCount: make(map[string]int),
		PrimaryDept: make(map[int]string),
		children:    make(map[string][]string),
	}
	for _, d := range depts {
		attribution.DeptNames[d.DeptId] = d.Name
		attribution.children[d.ParentId] = append(attribution.children[d.ParentId], d.DeptId)
	}
	for _, m := range members {
		if m.UserId <= 0 {
			continue
		}
		deptIds := memberDeptIds(&m)
		if len(deptIds) == 0 {
			continue
		}
		attribution.BoundUserIds = append(attribution.BoundUserIds, m.UserId)
		attribution.PrimaryDept[m.UserId] = deptIds[0]
		attribution.MemberCount[deptIds[0]]++
		attribution.members = append(attribution.members, orgMemberLite{UserId: m.UserId, deptIds: deptIds})
	}
	return attribution, nil
}
