package oauth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// 钉钉通讯录 v2 接口都挂在 oapi.dingtalk.com/topapi 下,POST + access_token query。
const dingTalkOrgAPIBase = "https://oapi.dingtalk.com/topapi/v2"

// dingTalkRootDeptId 根部门固定为 1,全体成员都直属根部门。
const dingTalkRootDeptId = 1

type dingTalkOrgAPIError struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (e *dingTalkOrgAPIError) check(api string) error {
	if e.ErrCode != 0 {
		return fmt.Errorf("dingtalk %s errcode=%d errmsg=%s", api, e.ErrCode, e.ErrMsg)
	}
	return nil
}

// dingTalkDeptInfo 是 department/listsub 返回的部门项。
type dingTalkDeptInfo struct {
	DeptId   int64  `json:"dept_id"`
	ParentId int64  `json:"parent_id"`
	Name     string `json:"name"`
}

// dingTalkDeptUser 是 user/list 返回的成员项。leader 表示该成员在所查部门内
// 是否主管(钉钉的主管标记挂在成员上,按部门区分)。mobile/email 未申请
// 高敏感权限时不返回,本功能也不读取。
type dingTalkDeptUser struct {
	UserId  string `json:"userid"`
	UnionId string `json:"unionid"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	Leader  bool   `json:"leader"`
}

func (p *DingTalkProvider) dingTalkOrgPost(ctx context.Context, token string, api string, body any, result any) error {
	bodyBytes, err := common.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		dingTalkOrgAPIBase+api+"?access_token="+url.QueryEscape(token), bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
	// 先解错误头校验 errcode(权限缺失/限流在这里现形),再解业务字段。
	var head dingTalkOrgAPIError
	if err = common.Unmarshal(data, &head); err != nil {
		return err
	}
	if err = head.check(api); err != nil {
		return err
	}
	return common.Unmarshal(data, result)
}

// listDingTalkSubDepts 返回某部门的直属子部门(listsub 无分页,只有下一级)。
func (p *DingTalkProvider) listDingTalkSubDepts(ctx context.Context, token string, deptId int64) ([]dingTalkDeptInfo, error) {
	var resp struct {
		Result []dingTalkDeptInfo `json:"result"`
	}
	err := p.dingTalkOrgPost(ctx, token, "/department/listsub",
		map[string]any{"dept_id": deptId, "language": "zh_CN"}, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// listDingTalkDeptUsers 游标分页拉取一个部门的直属成员。
func (p *DingTalkProvider) listDingTalkDeptUsers(ctx context.Context, token string, deptId int64, requestGap time.Duration) ([]dingTalkDeptUser, error) {
	var users []dingTalkDeptUser
	cursor := 0
	for {
		var resp struct {
			Result struct {
				HasMore    bool               `json:"has_more"`
				NextCursor int64              `json:"next_cursor"`
				List       []dingTalkDeptUser `json:"list"`
			} `json:"result"`
		}
		err := p.dingTalkOrgPost(ctx, token, "/user/list",
			map[string]any{"dept_id": deptId, "cursor": cursor, "size": 100, "language": "zh_CN"},
			&resp)
		if err != nil {
			return nil, err
		}
		users = append(users, resp.Result.List...)
		if !resp.Result.HasMore {
			return users, nil
		}
		cursor = int(resp.Result.NextCursor)
		if requestGap > 0 {
			time.Sleep(requestGap)
		}
	}
}

// FetchDingTalkOrgSnapshot 全量拉取部门树与成员,组装成快照模型。
// 部门主管取成员级 leader 标记聚合(不依赖管理员在钉钉后台设置的
// dept_manager_userid_list,那个可能为空)。任一 API 失败直接报错,
// 调用方必须保留旧快照,绝不用半成品覆盖。
func (p *DingTalkProvider) FetchDingTalkOrgSnapshot(ctx context.Context, requestGap time.Duration) ([]model.OrgDepartment, []model.OrgMember, error) {
	token, err := p.GetAppAccessToken(ctx)
	if err != nil {
		return nil, nil, err
	}

	// 1. 从根部门广度优先遍历整棵树(listsub 只返回直属下一级,需逐级展开)。
	deptInfos := []dingTalkDeptInfo{{DeptId: dingTalkRootDeptId, ParentId: 0, Name: "全体成员"}}
	queue := []int64{dingTalkRootDeptId}
	seen := map[int64]bool{dingTalkRootDeptId: true}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		children, err := p.listDingTalkSubDepts(ctx, token, parent)
		if err != nil {
			return nil, nil, err
		}
		for _, child := range children {
			if seen[child.DeptId] {
				continue
			}
			seen[child.DeptId] = true
			deptInfos = append(deptInfos, child)
			queue = append(queue, child.DeptId)
		}
		if requestGap > 0 && len(queue) > 0 {
			time.Sleep(requestGap)
		}
	}

	// 2. 逐部门拉成员,按 unionId 合并多部门归属与主管标记。
	type memberAccum struct {
		userId        string
		name          string
		title         string
		deptIds       []string
		leaderDeptIds []string
		deptSeen      map[string]bool
	}
	members := make(map[string]*memberAccum)
	for _, dept := range deptInfos {
		users, err := p.listDingTalkDeptUsers(ctx, token, dept.DeptId, requestGap)
		if err != nil {
			return nil, nil, err
		}
		deptKey := strconv.FormatInt(dept.DeptId, 10)
		for _, u := range users {
			if u.UnionId == "" {
				continue
			}
			acc, ok := members[u.UnionId]
			if !ok {
				acc = &memberAccum{deptSeen: make(map[string]bool)}
				members[u.UnionId] = acc
			}
			if acc.userId == "" {
				acc.userId = u.UserId
				acc.name = u.Name
				acc.title = u.Title
			}
			if !acc.deptSeen[deptKey] {
				acc.deptSeen[deptKey] = true
				acc.deptIds = append(acc.deptIds, deptKey)
				if u.Leader {
					acc.leaderDeptIds = append(acc.leaderDeptIds, deptKey)
				}
			}
		}
		if requestGap > 0 {
			time.Sleep(requestGap)
		}
	}

	// 3. 组装快照:部门 member_count 由成员归属反推,保持一致口径。
	memberCount := make(map[string]int)
	for _, acc := range members {
		for _, deptKey := range acc.deptIds {
			memberCount[deptKey]++
		}
	}
	depts := make([]model.OrgDepartment, 0, len(deptInfos))
	for i, info := range deptInfos {
		deptKey := strconv.FormatInt(info.DeptId, 10)
		parentKey := ""
		if info.DeptId != dingTalkRootDeptId {
			parentKey = strconv.FormatInt(info.ParentId, 10)
		}
		depts = append(depts, model.OrgDepartment{
			Provider:    model.OrgProviderDingTalk,
			DeptId:      deptKey,
			ParentId:    parentKey,
			Name:        info.Name,
			MemberCount: memberCount[deptKey],
			SortOrder:   i,
		})
	}
	orgMembers := make([]model.OrgMember, 0, len(members))
	// 排序输出,保证快照行序稳定,便于排查 diff。
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
			Provider:       model.OrgProviderDingTalk,
			UnionId:        unionId,
			ProviderUserId: acc.userId,
			Name:           acc.name,
			Title:          acc.title,
			DeptIds:        string(deptIdsJson),
			LeaderDeptIds:  string(leaderDeptIdsJson),
		})
	}
	return depts, orgMembers, nil
}
