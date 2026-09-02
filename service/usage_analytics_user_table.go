package service

import (
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/model"
)

// 用户分析表格的活跃状态: active=范围内有 consume 请求; silent=已绑定用户范围内
// 无请求; never=组织快照里有但从未绑定/登录的成员(UserId=0)。
const (
	AnalyticsUserStatusActive = "active"
	AnalyticsUserStatusSilent = "silent"
	AnalyticsUserStatusNever  = "never"
)

// AnalyticsUserTableEntry 用户分析表格行。UserId=0 表示未绑定成员,
// 此时 MemberKey 为带前缀的 unionId,保证前端行 key 稳定且不与数字 id 冲突。
type AnalyticsUserTableEntry struct {
	UserId         int     `json:"user_id"`
	MemberKey      string  `json:"member_key"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"display_name"`
	DeptName       string  `json:"dept_name"`
	Status         string  `json:"status"`
	RequestCount   int64   `json:"request_count"`
	FailCount      int64   `json:"fail_count"`
	Quota          int64   `json:"quota"`
	NetQuota       int64   `json:"net_quota"`
	ActiveDays     int64   `json:"active_days"`
	LastActiveDate string  `json:"last_active_date"`
	TopModel       string  `json:"top_model"`
	AvgUseTime     float64 `json:"avg_use_time"` // 秒, total_use_time / request_count(同为 consume-only 口径)
}

// BuildAnalyticsUserTable 构造用户分析全量表(含零活跃用户)。全集来源:
//   - admin + 已配置组织同步: 快照全部成员(含未绑定) ∪ 有统计但不在名册的用户;
//   - admin + 未配置组织同步: 本地未删用户(GetUserIdentities),只有 active/silent
//     两态,不可能出现 never(没有名册就无从知道"谁还没来");
//   - dept: scope.UserIds(已含零活跃绑定成员) + 子树内未绑定成员(内存匹配,
//     DeptIds 是 JSON 文本无索引,成员量几百可接受)。
func BuildAnalyticsUserTable(scope *AnalyticsScope, startDate, endDate string) ([]AnalyticsUserTableEntry, error) {
	statsRows, err := model.QueryUsageUserTable(startDate, endDate, scope.UserIds)
	if err != nil {
		return nil, err
	}
	statsByUser := make(map[int]model.UsageUserTableRow, len(statsRows))
	for _, r := range statsRows {
		statsByUser[r.UserId] = r
	}
	topModels, err := topModelPerUser(startDate, endDate, scope.UserIds)
	if err != nil {
		return nil, err
	}

	provider := ActiveOrgSyncProvider()
	entries := []AnalyticsUserTableEntry{}
	deptNameByUser := map[int]string{}
	inUniverse := map[int]bool{}

	addBound := func(uid int) {
		inUniverse[uid] = true
		entries = append(entries, statsToEntry(uid, statsByUser, topModels))
	}

	if scope.Scope == "admin" && provider == "" {
		users, err := model.GetUserIdentities()
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			addBound(u.Id)
		}
	} else {
		attribution, err := LoadOrgDeptAttribution(provider)
		if err != nil {
			return nil, err
		}
		members, err := model.GetOrgMembers(provider)
		if err != nil {
			return nil, err
		}
		if scope.Scope == "admin" {
			for uid, deptId := range attribution.PrimaryDept {
				deptNameByUser[uid] = attribution.DeptNames[deptId]
			}
			// 直接遍历成员(而不是 BoundUserIds): DeptIds 为空的已绑定成员
			// 不在归因里,但仍是全集的一部分。
			for _, m := range members {
				if m.UserId > 0 {
					addBound(m.UserId)
				} else {
					// admin 视野无子树限制,未绑定成员部门直接取 DeptIds[0]。
					deptName := ""
					if deptIds := memberDeptIds(&m); len(deptIds) > 0 {
						deptName = attribution.DeptNames[deptIds[0]]
					}
					entries = append(entries, neverEntry(&m, deptName))
				}
			}
			// 有统计但不在名册的用户(快照外账号,如本地注册的 root)不能漏。
			for uid := range statsByUser {
				if !inUniverse[uid] {
					addBound(uid)
				}
			}
		} else {
			// scope 对象有 120s 缓存且并发共享,全程只读不改。
			for uid, deptId := range scope.PrimaryDept {
				deptNameByUser[uid] = scope.DeptNames[deptId]
			}
			for _, uid := range scope.UserIds {
				addBound(uid)
			}
			subtree := make(map[string]bool, len(scope.DeptIds))
			for _, deptId := range scope.DeptIds {
				subtree[deptId] = true
			}
			for _, m := range members {
				if m.UserId > 0 {
					continue
				}
				for _, deptId := range memberDeptIds(&m) {
					if subtree[deptId] {
						entries = append(entries, neverEntry(&m, scope.DeptNames[deptId]))
						break
					}
				}
			}
		}
	}

	boundIds := make([]int, 0, len(inUniverse))
	for uid := range inUniverse {
		boundIds = append(boundIds, uid)
	}
	displayNames, err := model.GetUserDisplayNames(boundIds)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		e := &entries[i]
		if e.Status == AnalyticsUserStatusNever {
			continue // 真名已在 neverEntry 里取自通讯录
		}
		if name := displayNames[e.UserId]; name != "" {
			e.DisplayName = name
		}
		e.DeptName = deptNameByUser[e.UserId]
	}

	sort.Slice(entries, func(i, j int) bool {
		ri, rj := analyticsStatusRank(entries[i].Status), analyticsStatusRank(entries[j].Status)
		if ri != rj {
			return ri < rj
		}
		if entries[i].NetQuota != entries[j].NetQuota {
			return entries[i].NetQuota > entries[j].NetQuota
		}
		if entries[i].UserId != entries[j].UserId {
			return entries[i].UserId < entries[j].UserId
		}
		return entries[i].MemberKey < entries[j].MemberKey
	})
	return entries, nil
}

func analyticsStatusRank(status string) int {
	switch status {
	case AnalyticsUserStatusActive:
		return 0
	case AnalyticsUserStatusSilent:
		return 1
	default:
		return 2
	}
}

// statsToEntry 已绑定用户行: 范围内有 consume 为 active,否则 silent(零活跃但保留行)。
func statsToEntry(uid int, stats map[int]model.UsageUserTableRow, topModels map[int]string) AnalyticsUserTableEntry {
	entry := AnalyticsUserTableEntry{
		UserId:    uid,
		MemberKey: strconv.Itoa(uid),
		Status:    AnalyticsUserStatusSilent,
	}
	r, ok := stats[uid]
	if !ok {
		return entry
	}
	entry.Username = r.Username
	entry.RequestCount = r.RequestCount
	entry.FailCount = r.FailCount
	entry.Quota = r.Quota
	entry.NetQuota = r.Quota - r.RefundQuota
	entry.ActiveDays = r.ActiveDays
	entry.LastActiveDate = r.LastActiveDate
	entry.TopModel = topModels[uid]
	if r.RequestCount > 0 {
		entry.Status = AnalyticsUserStatusActive
		entry.AvgUseTime = float64(r.TotalUseTime) / float64(r.RequestCount)
	}
	return entry
}

// neverEntry 未绑定成员行: 真名取通讯录快照,没有任何使用统计。
func neverEntry(m *model.OrgMember, deptName string) AnalyticsUserTableEntry {
	return AnalyticsUserTableEntry{
		MemberKey:   "org:" + m.UnionId,
		DisplayName: m.Name,
		DeptName:    deptName,
		Status:      AnalyticsUserStatusNever,
	}
}

// topModelPerUser 每用户主力模型: 按 request_count desc → quota desc → model_name
// 字典序取 top1,定序确定性防跨请求抖动。只考虑有 consume 的模型行(纯失败
// 不算"在用")。
func topModelPerUser(startDate, endDate string, userIds []int) (map[int]string, error) {
	rows, err := model.QueryUsageByUserModel(startDate, endDate, userIds)
	if err != nil {
		return nil, err
	}
	type best struct {
		model    string
		requests int64
		quota    int64
	}
	bestByUser := make(map[int]best)
	for _, r := range rows {
		if r.RequestCount <= 0 {
			continue
		}
		b, ok := bestByUser[r.UserId]
		better := !ok || r.RequestCount > b.requests ||
			(r.RequestCount == b.requests && r.Quota > b.quota) ||
			(r.RequestCount == b.requests && r.Quota == b.quota && r.ModelName < b.model)
		if better {
			bestByUser[r.UserId] = best{model: r.ModelName, requests: r.RequestCount, quota: r.Quota}
		}
	}
	out := make(map[int]string, len(bestByUser))
	for uid, b := range bestByUser {
		out[uid] = b.model
	}
	return out, nil
}
