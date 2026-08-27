package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// skill 候选状态。
const (
	SkillCandidateStatusPending   = "pending"
	SkillCandidateStatusPublished = "published"
	SkillCandidateStatusRejected  = "rejected"
)

// Skill 库条目状态。
const (
	SkillStatusPublished = "published"
	SkillStatusArchived  = "archived"
)

// skillCandidateMaxUserIds 候选记录里去重用户列表的存储上限,防超长 JSON。
const skillCandidateMaxUserIds = 200

// SkillCandidate LLM 分类沉淀出的 skill 候选,管理员审核后才进 Skill 库。
type SkillCandidate struct {
	Id          int    `json:"id"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint"`
	Title       string `json:"title" gorm:"type:varchar(128);uniqueIndex"`
	Category    string `json:"category" gorm:"type:varchar(32);index;default:''"`
	SamplePrompt string `json:"sample_prompt" gorm:"type:text"` // 截断样本 prompt
	OccurrenceCount int `json:"occurrence_count" gorm:"default:0"`
	// UserIds 出现过的用户去重列表(JSON 数组,上限 skillCandidateMaxUserIds),
	// UserCount 取其长度,避免为计数单独建关联表。
	UserIds          string `json:"user_ids" gorm:"type:text"`
	UserCount        int    `json:"user_count" gorm:"default:0"`
	Status           string `json:"status" gorm:"type:varchar(16);index;default:'pending'"`
	PublishedSkillId int    `json:"published_skill_id" gorm:"default:0"`
}

func (SkillCandidate) TableName() string { return "skill_candidates" }

// Skill 已发布(或已下架)的 skill 库条目。
type Skill struct {
	Id           int    `json:"id"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
	Title        string `json:"title" gorm:"type:varchar(128);index"`
	Category     string `json:"category" gorm:"type:varchar(32);index;default:''"`
	Description  string `json:"description" gorm:"type:text"`
	SamplePrompt string `json:"sample_prompt" gorm:"type:text"`
	Status       string `json:"status" gorm:"type:varchar(16);index;default:'published'"`
}

func (Skill) TableName() string { return "skills" }

// UpsertSkillCandidate 按标题归一化合并候选:已存在则累加次数/用户,并刷新
// 分类与样本(仅当新值非空)。返回是否为新创建的候选。
func UpsertSkillCandidate(title string, category string, samplePrompt string, userId int, now int64) (created bool, err error) {
	var candidate SkillCandidate
	tx := DB.Where("title = ?", title).First(&candidate)
	if tx.Error != nil {
		if tx.Error != gorm.ErrRecordNotFound {
			return false, tx.Error
		}
		userIds := "[]"
		if userId != 0 {
			data, _ := common.Marshal([]int{userId})
			userIds = string(data)
		}
		userCount := 0
		if userId != 0 {
			userCount = 1
		}
		candidate = SkillCandidate{
			CreatedAt: now, UpdatedAt: now,
			Title: title, Category: category, SamplePrompt: samplePrompt,
			OccurrenceCount: 1, UserIds: userIds, UserCount: userCount,
			Status: SkillCandidateStatusPending,
		}
		return true, DB.Create(&candidate).Error
	}

	var userIds []int
	_ = common.UnmarshalJsonStr(candidate.UserIds, &userIds)
	seen := false
	for _, id := range userIds {
		if id == userId {
			seen = true
			break
		}
	}
	if userId != 0 && !seen && len(userIds) < skillCandidateMaxUserIds {
		userIds = append(userIds, userId)
	}
	userIdsData, _ := common.Marshal(userIds)
	updates := map[string]any{
		"occurrence_count": candidate.OccurrenceCount + 1,
		"user_ids":         string(userIdsData),
		"user_count":       len(userIds),
		"updated_at":       now,
	}
	if category != "" && candidate.Category != category {
		updates["category"] = category
	}
	if samplePrompt != "" && candidate.SamplePrompt == "" {
		updates["sample_prompt"] = samplePrompt
	}
	return false, DB.Model(&candidate).Updates(updates).Error
}

// GetSkillCandidates 管理端分页查询候选,status 为空查全部。
func GetSkillCandidates(status string, keyword string, startIdx int, num int) (items []*SkillCandidate, total int64, err error) {
	tx := DB.Model(&SkillCandidate{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if keyword != "" {
		tx = tx.Where("title LIKE ? OR category LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = tx.Order("updated_at desc, id desc").Limit(num).Offset(startIdx).Find(&items).Error
	return items, total, err
}

func GetSkillCandidateById(id int) (*SkillCandidate, error) {
	var candidate SkillCandidate
	if err := DB.First(&candidate, id).Error; err != nil {
		return nil, err
	}
	return &candidate, nil
}

// PublishSkillCandidate 审核通过:建 Skill 条目并把候选标记为 published。
// 同事务保证两张表状态一致。
func PublishSkillCandidate(candidate *SkillCandidate, title string, category string, description string, now int64) (*Skill, error) {
	skill := &Skill{
		CreatedAt: now, UpdatedAt: now,
		Title: title, Category: category, Description: description,
		SamplePrompt: candidate.SamplePrompt, Status: SkillStatusPublished,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(skill).Error; err != nil {
			return err
		}
		return tx.Model(candidate).Updates(map[string]any{
			"status":             SkillCandidateStatusPublished,
			"published_skill_id": skill.Id,
			"updated_at":         now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return skill, nil
}

// RejectSkillCandidate 审核拒绝。
func RejectSkillCandidate(id int, now int64) error {
	return DB.Model(&SkillCandidate{}).Where("id = ?", id).Updates(map[string]any{
		"status": SkillCandidateStatusRejected, "updated_at": now,
	}).Error
}

// GetSkills 管理端分页查询 skill 库,status 为空查全部。
func GetSkills(status string, keyword string, startIdx int, num int) (items []*Skill, total int64, err error) {
	tx := DB.Model(&Skill{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if keyword != "" {
		tx = tx.Where("title LIKE ? OR category LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = tx.Order("updated_at desc, id desc").Limit(num).Offset(startIdx).Find(&items).Error
	return items, total, err
}

// UpdateSkill 编辑 skill 条目(标题/分类/描述/样本)。
func UpdateSkill(id int, title string, category string, description string, samplePrompt string, now int64) error {
	return DB.Model(&Skill{}).Where("id = ?", id).Updates(map[string]any{
		"title": title, "category": category, "description": description,
		"sample_prompt": samplePrompt, "updated_at": now,
	}).Error
}

// ArchiveSkill 下架(不出现在库列表默认视图,数据保留)。
func ArchiveSkill(id int, now int64) error {
	return DB.Model(&Skill{}).Where("id = ?", id).Updates(map[string]any{
		"status": SkillStatusArchived, "updated_at": now,
	}).Error
}
