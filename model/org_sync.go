package model

import (
	"errors"

	"gorm.io/gorm"
)

// ErrOrgProviderUnsupported 同步来源不是内置的 dingtalk/feishu。
var ErrOrgProviderUnsupported = errors.New("unsupported org provider")

// 组织架构同步的来源平台。与 OAuth provider 名保持一致；钉钉/飞书互斥,
// 任一时刻只有一方启用,快照按 provider 隔离,互不影响。
const (
	OrgProviderDingTalk = "dingtalk"
	OrgProviderFeishu   = "feishu"
)

// OrgDepartment 组织架构部门快照。每次同步对同一 provider 全量替换,
// 不自增关联 members,查询时按 provider + dept_id 关联。
type OrgDepartment struct {
	Id            int64  `json:"id" gorm:"primary_key"`
	Provider      string `json:"provider" gorm:"type:varchar(16);uniqueIndex:idx_org_dept_provider_dept,priority:1"`
	DeptId        string `json:"dept_id" gorm:"type:varchar(64);uniqueIndex:idx_org_dept_provider_dept,priority:2"` // provider 侧部门 id
	ParentId      string `json:"parent_id" gorm:"type:varchar(64);index"`                                           // 根部门为空串
	Name          string `json:"name" gorm:"type:varchar(128)"`
	LeaderUserIds string `json:"leader_user_ids" gorm:"type:text"` // 部门主管 unionId 列表(JSON 数组)
	MemberCount   int    `json:"member_count"`
	SortOrder     int    `json:"sort_order"`              // 同级排序(飞书 order;钉钉无此概念,恒 0)
	SyncedAt      int64  `json:"synced_at" gorm:"bigint"` // 本次快照时间
}

func (OrgDepartment) TableName() string { return "org_departments" }

// OrgMember 组织成员快照。UnionId 是匹配本地用户的键(与 users.dingtalk_id /
// feishu_id 存的值一致)。UserId=0 表示该成员尚未登录/绑定过本平台。
type OrgMember struct {
	Id             int64  `json:"id" gorm:"primary_key"`
	Provider       string `json:"provider" gorm:"type:varchar(16);uniqueIndex:idx_org_member_provider_union,priority:1"`
	UnionId        string `json:"union_id" gorm:"type:varchar(128);uniqueIndex:idx_org_member_provider_union,priority:2"`
	ProviderUserId string `json:"provider_user_id" gorm:"type:varchar(128)"` // 钉钉 userid / 飞书 open_id
	Name           string `json:"name" gorm:"type:varchar(128)"`
	Title          string `json:"title" gorm:"type:varchar(128)"`            // 职位
	DeptIds        string `json:"dept_ids" gorm:"type:text"`                 // 所属部门 id 列表(JSON 数组,一人可多部门)
	LeaderDeptIds  string `json:"leader_dept_ids" gorm:"type:text"`          // 在哪些部门是主管(JSON 数组)
	UserId         int    `json:"user_id" gorm:"index"`                      // 匹配到的本地用户,0=未绑定
	GroupMapped    bool   `json:"group_mapped"`                              // 分组是否由同步映射写入(区分管理员手动调整)
	PrevGroup      string `json:"prev_group" gorm:"type:varchar(64)"`        // 映射前的分组,卸任主管时恢复
	SyncedAt       int64  `json:"synced_at" gorm:"bigint"`

	// Username/DisplayName 由查询时按 UserId 补充,不落库
	Username    string `json:"username,omitempty" gorm:"-"`
	DisplayName string `json:"display_name,omitempty" gorm:"-"`
}

func (OrgMember) TableName() string { return "org_members" }

// OrgGroupChange 一次分组映射动作。ExpectCurrent 是乐观并发守卫:只有用户当前
// 分组仍等于期望值才执行更新,避免覆盖同步间隙管理员的手动调整。
type OrgGroupChange struct {
	UserId        int
	ExpectCurrent string
	NewGroup      string
}

// ReplaceOrgSnapshot 在一个事务内整体替换 provider 的组织快照,并应用分组
// 映射变更。返回实际生效了分组变更的用户 id 列表(用于提交后刷新分组缓存)。
func ReplaceOrgSnapshot(provider string, depts []OrgDepartment, members []OrgMember, groupChanges []OrgGroupChange) (appliedUserIds []int, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider = ?", provider).Delete(&OrgDepartment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("provider = ?", provider).Delete(&OrgMember{}).Error; err != nil {
			return err
		}
		if len(depts) > 0 {
			if err := tx.CreateInBatches(depts, 100).Error; err != nil {
				return err
			}
		}
		if len(members) > 0 {
			if err := tx.CreateInBatches(members, 100).Error; err != nil {
				return err
			}
		}
		for _, change := range groupChanges {
			// Where 用 commonGroupCol(原生 SQL 片段需手动转义保留字);
			// Update 的列名给裸 "group" 让 GORM 按方言自行加引号(同 subscription.go)。
			res := tx.Model(&User{}).
				Where("id = ? AND "+commonGroupCol+" = ?", change.UserId, change.ExpectCurrent).
				Update("group", change.NewGroup)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				appliedUserIds = append(appliedUserIds, change.UserId)
			}
		}
		return nil
	})
	return appliedUserIds, err
}

// GetOrgDepartments 返回 provider 的部门快照,按父部门+排序+id 排列,
// 前端据 ParentId 组树。
func GetOrgDepartments(provider string) ([]OrgDepartment, error) {
	var depts []OrgDepartment
	err := DB.Where("provider = ?", provider).
		Order("parent_id asc, sort_order asc, id asc").
		Find(&depts).Error
	return depts, err
}

// GetOrgMembers 返回 provider 的成员快照。顺带按 UserId 批量补充本地
// username/display_name,未绑定的保持空。
func GetOrgMembers(provider string) ([]OrgMember, error) {
	var members []OrgMember
	if err := DB.Where("provider = ?", provider).Order("id asc").Find(&members).Error; err != nil {
		return nil, err
	}
	userIds := make([]int, 0, len(members))
	for _, m := range members {
		if m.UserId > 0 {
			userIds = append(userIds, m.UserId)
		}
	}
	if len(userIds) == 0 {
		return members, nil
	}
	var users []User
	if err := DB.Select("id", "username", "display_name").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	byId := make(map[int]User, len(users))
	for _, u := range users {
		byId[u.Id] = u
	}
	for i := range members {
		if u, ok := byId[members[i].UserId]; ok {
			members[i].Username = u.Username
			members[i].DisplayName = u.DisplayName
		}
	}
	return members, nil
}

// GetOrgMemberGroupStates 读取 provider 现有成员的分组映射状态
// (unionId -> 是否已映射/映射前分组),供同步引擎在替换快照前计算映射动作。
func GetOrgMemberGroupStates(provider string) (map[string]OrgMember, error) {
	var members []OrgMember
	if err := DB.Select("union_id", "user_id", "group_mapped", "prev_group").
		Where("provider = ?", provider).Find(&members).Error; err != nil {
		return nil, err
	}
	states := make(map[string]OrgMember, len(members))
	for _, m := range members {
		states[m.UnionId] = m
	}
	return states, nil
}

// GetUserIdsByOrgUnionIds 按 unionId 批量匹配本地用户。provider 决定查
// users 表的 dingtalk_id 还是 feishu_id 列(列名来自固定常量,无注入面)。
func GetUserIdsByOrgUnionIds(provider string, unionIds []string) (map[string]int, error) {
	if len(unionIds) == 0 {
		return map[string]int{}, nil
	}
	column := ""
	switch provider {
	case OrgProviderDingTalk:
		column = "dingtalk_id"
	case OrgProviderFeishu:
		column = "feishu_id"
	default:
		return nil, ErrOrgProviderUnsupported
	}
	var users []User
	if err := DB.Select("id", column).Where(column+" IN ?", unionIds).Find(&users).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int, len(users))
	for _, u := range users {
		switch provider {
		case OrgProviderDingTalk:
			result[u.DingTalkId] = u.Id
		case OrgProviderFeishu:
			result[u.FeishuId] = u.Id
		}
	}
	return result, nil
}

// BindOrgMemberToUser 登录即绑定:OAuth 登录/绑定拿到 unionId 后立刻把
// 快照里对应成员行的 UserId 补齐,不必等下一次全量同步。成员尚未进快照
// (上次同步后才加入通讯录)时无匹配行,不算错误,等下次同步自然建立;
// 已绑定到同一用户的行跳过不写。分组映射不在此处理,仍由同步统一计算。
func BindOrgMemberToUser(provider, unionId string, userId int) error {
	if unionId == "" || userId <= 0 {
		return nil
	}
	switch provider {
	case OrgProviderDingTalk, OrgProviderFeishu:
	default:
		return ErrOrgProviderUnsupported
	}
	return DB.Model(&OrgMember{}).
		Where("provider = ? AND union_id = ? AND user_id <> ?", provider, unionId, userId).
		Update("user_id", userId).Error
}

// GetOrgUserGroups 批量读取用户当前分组,供分组映射计算期望值。
func GetOrgUserGroups(userIds []int) (map[int]string, error) {
	if len(userIds) == 0 {
		return map[int]string{}, nil
	}
	type row struct {
		Id    int
		Group string
	}
	var rows []row
	if err := DB.Model(&User{}).Select("id", commonGroupCol+" AS "+commonGroupCol).
		Where("id IN ?", userIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[int]string, len(rows))
	for _, r := range rows {
		result[r.Id] = r.Group
	}
	return result, nil
}
