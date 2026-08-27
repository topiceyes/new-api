package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertSkillCandidateCreateAndMerge(t *testing.T) {
	truncateTables(t)

	created, err := UpsertSkillCandidate("SQL 优化", "programming", "帮我优化这条 SQL", 101, 1000)
	require.NoError(t, err)
	assert.True(t, created)

	// 同标题同用户再次上报:只累加次数,用户不重复。
	created, err = UpsertSkillCandidate("SQL 优化", "programming", "", 101, 1001)
	require.NoError(t, err)
	assert.False(t, created)

	// 同标题不同用户:用户数累加。
	created, err = UpsertSkillCandidate("SQL 优化", "programming", "", 102, 1002)
	require.NoError(t, err)
	assert.False(t, created)

	var candidate SkillCandidate
	require.NoError(t, DB.Where("title = ?", "SQL 优化").First(&candidate).Error)
	assert.Equal(t, 3, candidate.OccurrenceCount)
	assert.Equal(t, 2, candidate.UserCount)
	assert.Equal(t, "[101,102]", candidate.UserIds)
	assert.Equal(t, SkillCandidateStatusPending, candidate.Status)
	assert.Equal(t, "programming", candidate.Category)
	assert.Equal(t, "帮我优化这条 SQL", candidate.SamplePrompt)
	assert.Equal(t, int64(1002), candidate.UpdatedAt)
}

func TestUpsertSkillCandidateAnonymousUser(t *testing.T) {
	truncateTables(t)

	created, err := UpsertSkillCandidate("周报润色", "document", "润色周报", 0, 1000)
	require.NoError(t, err)
	assert.True(t, created)

	// userId=0(未知用户)不应进入用户列表,也不应增加 user_count。
	_, err = UpsertSkillCandidate("周报润色", "document", "", 0, 1001)
	require.NoError(t, err)

	var candidate SkillCandidate
	require.NoError(t, DB.Where("title = ?", "周报润色").First(&candidate).Error)
	assert.Equal(t, 2, candidate.OccurrenceCount)
	assert.Equal(t, 0, candidate.UserCount)
	assert.Equal(t, "[]", candidate.UserIds)
}

func TestPublishAndRejectSkillCandidate(t *testing.T) {
	truncateTables(t)

	_, err := UpsertSkillCandidate("接口联调", "programming", "帮我写联调请求", 201, 1000)
	require.NoError(t, err)
	var candidate SkillCandidate
	require.NoError(t, DB.Where("title = ?", "接口联调").First(&candidate).Error)

	// 审核通过:同事务建 Skill + 候选标记 published。
	skill, err := PublishSkillCandidate(&candidate, "接口联调助手", "programming", "生成联调请求体", 2000)
	require.NoError(t, err)
	require.NotZero(t, skill.Id)
	assert.Equal(t, "接口联调助手", skill.Title)
	assert.Equal(t, SkillStatusPublished, skill.Status)
	assert.Equal(t, candidate.SamplePrompt, skill.SamplePrompt)

	require.NoError(t, DB.First(&candidate, candidate.Id).Error)
	assert.Equal(t, SkillCandidateStatusPublished, candidate.Status)
	assert.Equal(t, skill.Id, candidate.PublishedSkillId)

	// 另一条走拒绝。
	_, err = UpsertSkillCandidate("闲聊", "chat", "今天天气", 201, 1000)
	require.NoError(t, err)
	var rejectedCandidate SkillCandidate
	require.NoError(t, DB.Where("title = ?", "闲聊").First(&rejectedCandidate).Error)
	require.NoError(t, RejectSkillCandidate(rejectedCandidate.Id, 2000))
	require.NoError(t, DB.First(&rejectedCandidate, rejectedCandidate.Id).Error)
	assert.Equal(t, SkillCandidateStatusRejected, rejectedCandidate.Status)

	// 分页查询:pending 已清空,published 能按状态过滤。
	items, total, err := GetSkillCandidates(SkillCandidateStatusPending, "", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, items)

	items, total, err = GetSkillCandidates("", "联调", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "接口联调", items[0].Title)
}

func TestSkillLibraryUpdateAndArchive(t *testing.T) {
	truncateTables(t)

	_, err := UpsertSkillCandidate("数据透视", "data", "按地区透视销量", 301, 1000)
	require.NoError(t, err)
	var candidate SkillCandidate
	require.NoError(t, DB.Where("title = ?", "数据透视").First(&candidate).Error)
	skill, err := PublishSkillCandidate(&candidate, "数据透视", "data", "", 2000)
	require.NoError(t, err)

	require.NoError(t, UpdateSkill(skill.Id, "数据透视分析", "data", "生成透视表思路", "新样本", 3000))
	var updated Skill
	require.NoError(t, DB.First(&updated, skill.Id).Error)
	assert.Equal(t, "数据透视分析", updated.Title)
	assert.Equal(t, "生成透视表思路", updated.Description)
	assert.Equal(t, int64(3000), updated.UpdatedAt)

	require.NoError(t, ArchiveSkill(skill.Id, 4000))
	require.NoError(t, DB.First(&updated, skill.Id).Error)
	assert.Equal(t, SkillStatusArchived, updated.Status)

	// 归档后按 published 过滤查不到,不带状态过滤仍在。
	_, total, err := GetSkills(SkillStatusPublished, "", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	_, total, err = GetSkills("", "", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}
