package oauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// 飞书通讯录 v3 接口都挂在 open.feishu.cn/open-apis/contact/v3 下,
// GET + Authorization: Bearer <tenant_access_token>。
const feishuOrgAPIBase = "https://open.feishu.cn/open-apis/contact/v3"

// feishuRootDeptId 根部门 id 为 "0",全体成员都直属根部门。
const feishuRootDeptId = "0"

// feishuOrgDepartment 是 departments children 返回的部门项。leader_user_id
// 是部门负责人(飞书的主管标记挂在部门上),id 类型随 user_id_type 参数,
// 本功能统一用 union_id 以便与 users.feishu_id 绑定列匹配。
type feishuOrgDepartment struct {
	DepartmentId       string `json:"department_id"`
	ParentDepartmentId string `json:"parent_department_id"`
	Name               string `json:"name"`
	LeaderUserId       string `json:"leader_user_id"`
	Order              int    `json:"order"`
	Status             struct {
		IsDeleted bool `json:"is_deleted"`
	} `json:"status"`
}

// feishuOrgUser 是 users/find_by_department 返回的成员项。注意用户对象上的
// leader_user_id 是「直属上级」(汇报线),不是「此人是主管」,这里不读它;
// 主管判定只用部门对象的 leader_user_id 反查。
type feishuOrgUser struct {
	UnionId       string   `json:"union_id"`
	OpenId        string   `json:"open_id"`
	Name          string   `json:"name"`
	JobTitle      string   `json:"job_title"`
	DepartmentIds []string `json:"department_ids"`
	Status        feishuUserStatus `json:"status"`
}

func (p *FeishuProvider) feishuOrgGet(ctx context.Context, token string, path string, query url.Values, result any) error {
	endpoint := feishuOrgAPIBase + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	// 先解错误头:权限范围不足(41050)、scope 缺失等在这里现形。
	var head feishuAPIError
	if err = common.Unmarshal(data, &head); err != nil {
		return err
	}
	if head.Code != 0 {
		return fmt.Errorf("feishu %s code=%d msg=%s", path, head.Code, head.Msg)
	}
	return common.Unmarshal(data, result)
}

// listFeishuDepartments 从根部门一次性递归拉全树(fetch_child=true),
// 分页上限 50,返回不含根部门本身。
func (p *FeishuProvider) listFeishuDepartments(ctx context.Context, token string, requestGap time.Duration) ([]feishuOrgDepartment, error) {
	var depts []feishuOrgDepartment
	pageToken := ""
	for {
		query := url.Values{
			"department_id_type": {"department_id"},
			"user_id_type":       {"union_id"},
			"fetch_child":        {"true"},
			"page_size":          {"50"},
		}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var resp struct {
			Data struct {
				HasMore   bool                 `json:"has_more"`
				PageToken string               `json:"page_token"`
				Items     []feishuOrgDepartment `json:"items"`
			} `json:"data"`
		}
		if err := p.feishuOrgGet(ctx, token, "/departments/"+feishuRootDeptId+"/children", query, &resp); err != nil {
			return nil, err
		}
		for _, dept := range resp.Data.Items {
			if dept.Status.IsDeleted {
				continue
			}
			depts = append(depts, dept)
		}
		if !resp.Data.HasMore {
			return depts, nil
		}
		pageToken = resp.Data.PageToken
		if requestGap > 0 {
			time.Sleep(requestGap)
		}
	}
}

// listFeishuDeptUsers 分页拉取一个部门的直属成员(只返回在职)。
func (p *FeishuProvider) listFeishuDeptUsers(ctx context.Context, token string, deptId string, requestGap time.Duration) ([]feishuOrgUser, error) {
	var users []feishuOrgUser
	pageToken := ""
	for {
		query := url.Values{
			"department_id":      {deptId},
			"department_id_type": {"department_id"},
			"user_id_type":       {"union_id"},
			"page_size":          {"50"},
		}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var resp struct {
			Data struct {
				HasMore   bool           `json:"has_more"`
				PageToken string         `json:"page_token"`
				Items     []feishuOrgUser `json:"items"`
			} `json:"data"`
		}
		if err := p.feishuOrgGet(ctx, token, "/users/find_by_department", query, &resp); err != nil {
			return nil, err
		}
		users = append(users, resp.Data.Items...)
		if !resp.Data.HasMore {
			return users, nil
		}
		pageToken = resp.Data.PageToken
		if requestGap > 0 {
			time.Sleep(requestGap)
		}
	}
}

// FetchFeishuOrgSnapshot 全量拉取部门树与成员,组装成快照模型。
// 任一 API 失败直接报错,调用方必须保留旧快照,绝不用半成品覆盖。
func (p *FeishuProvider) FetchFeishuOrgSnapshot(ctx context.Context, requestGap time.Duration) ([]model.OrgDepartment, []model.OrgMember, error) {
	token, err := p.GetTenantAccessToken(ctx)
	if err != nil {
		return nil, nil, err
	}

	deptInfos, err := p.listFeishuDepartments(ctx, token, requestGap)
	if err != nil {
		return nil, nil, err
	}
	// 补上根部门(children 不含根本身),成员数稍后由归属反推。
	deptInfos = append([]feishuOrgDepartment{{
		DepartmentId: feishuRootDeptId,
		Name:         "全体成员",
	}}, deptInfos...)

	type memberAccum struct {
		openId        string
		name          string
		title         string
		deptIds       []string
		leaderDeptIds []string
		deptSeen      map[string]bool
	}
	members := make(map[string]*memberAccum)
	for _, dept := range deptInfos {
		users, err := p.listFeishuDeptUsers(ctx, token, dept.DepartmentId, requestGap)
		if err != nil {
			return nil, nil, err
		}
		for _, u := range users {
			if u.UnionId == "" {
				continue
			}
			// find_by_department 只返回在职,但冻结/已退出状态仍可能出现,
			// 与离职巡检口径一致:resigned/exited 一律不进快照。
			if u.Status.IsResigned || u.Status.IsExited {
				continue
			}
			acc, ok := members[u.UnionId]
			if !ok {
				acc = &memberAccum{deptSeen: make(map[string]bool)}
				members[u.UnionId] = acc
			}
			if acc.openId == "" {
				acc.openId = u.OpenId
				acc.name = u.Name
				acc.title = u.JobTitle
			}
			if !acc.deptSeen[dept.DepartmentId] {
				acc.deptSeen[dept.DepartmentId] = true
				acc.deptIds = append(acc.deptIds, dept.DepartmentId)
			}
		}
		if dept.LeaderUserId != "" {
			acc, ok := members[dept.LeaderUserId]
			if ok && !containsString(acc.leaderDeptIds, dept.DepartmentId) {
				acc.leaderDeptIds = append(acc.leaderDeptIds, dept.DepartmentId)
			}
		}
		if requestGap > 0 {
			time.Sleep(requestGap)
		}
	}

	memberCount := make(map[string]int)
	for _, acc := range members {
		for _, deptId := range acc.deptIds {
			memberCount[deptId]++
		}
	}
	depts := make([]model.OrgDepartment, 0, len(deptInfos))
	for _, info := range deptInfos {
		parentId := info.ParentDepartmentId
		if info.DepartmentId == feishuRootDeptId {
			parentId = ""
		}
		leaderIds := "[]"
		if info.LeaderUserId != "" {
			leaderJson, _ := common.Marshal([]string{info.LeaderUserId})
			leaderIds = string(leaderJson)
		}
		depts = append(depts, model.OrgDepartment{
			Provider:      model.OrgProviderFeishu,
			DeptId:        info.DepartmentId,
			ParentId:      parentId,
			Name:          info.Name,
			LeaderUserIds: leaderIds,
			MemberCount:   memberCount[info.DepartmentId],
			SortOrder:     info.Order,
		})
	}
	orgMembers := make([]model.OrgMember, 0, len(members))
	unionIds := make([]string, 0, len(members))
	for unionId := range members {
		unionIds = append(unionIds, unionId)
	}
	sort.Strings(unionIds)
	for _, unionId := range unionIds {
		acc := members[unionId]
		deptIdsJson, _ := common.Marshal(acc.deptIds)
		leaderDeptIdsJson, _ := common.Marshal(acc.leaderDeptIds)
		orgMembers = append(orgMembers, model.OrgMember{
			Provider:       model.OrgProviderFeishu,
			UnionId:        unionId,
			ProviderUserId: acc.openId,
			Name:           acc.name,
			Title:          acc.title,
			DeptIds:        string(deptIdsJson),
			LeaderDeptIds:  string(leaderDeptIdsJson),
		})
	}
	return depts, orgMembers, nil
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
