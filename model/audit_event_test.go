package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAuditEvents(t *testing.T) {
	t.Helper()
	truncateTables(t)
	events := []*AuditEvent{
		{CreatedAt: 1000, EventType: AuditEventTypePiiHit, Severity: "warning", UserId: 1, Username: "alice", TokenId: 11, TokenName: "prod-key", ModelName: "gpt-4o", RuleId: "builtin.id_card_cn", RuleName: "身份证号", Excerpt: "110****39", Prompt: "原文甲"},
		{CreatedAt: 2000, EventType: AuditEventTypePiiHit, Severity: "critical", UserId: 2, Username: "bob", TokenId: 22, TokenName: "test-key", ModelName: "claude-sonnet", RuleId: "builtin.api_key_sk", RuleName: "API 密钥 (sk-)", Excerpt: "sk-****yz"},
		{CreatedAt: 3000, EventType: AuditEventTypeKeyShareMultiIP, Severity: "warning", UserId: 1, Username: "alice", TokenId: 11, Detail: `{"distinct_ips":6}`},
	}
	for _, e := range events {
		require.NoError(t, CreateAuditEvent(e))
		require.NotZero(t, e.Id)
	}
}

func TestAuditEventCRUD(t *testing.T) {
	seedAuditEvents(t)

	// 详情接口返回 prompt 原文
	event, err := GetAuditEventById(1)
	require.NoError(t, err)
	require.Equal(t, "alice", event.Username)
	assert.Equal(t, "原文甲", event.Prompt)

	_, err = GetAuditEventById(9999)
	assert.Error(t, err)
}

func TestGetAuditEventsPaginationAndFilters(t *testing.T) {
	seedAuditEvents(t)

	// 列表接口不得返回 prompt 原文(Select 显式排除)
	events, total, err := GetAuditEvents("", "", 0, 0, "", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, events, 3)
	// 按 created_at 倒序
	assert.EqualValues(t, 3000, events[0].CreatedAt)
	for _, e := range events {
		assert.Empty(t, e.Prompt, "list query must not load prompt column")
	}

	// 分页
	page, total, err := GetAuditEvents("", "", 0, 0, "", 0, 0, 1, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, page, 1)
	assert.EqualValues(t, 2000, page[0].CreatedAt)

	// 事件类型过滤
	events, total, err = GetAuditEvents(AuditEventTypeKeyShareMultiIP, "", 0, 0, "", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, events, 1)
	assert.Equal(t, AuditEventTypeKeyShareMultiIP, events[0].EventType)

	// 严重度 + 用户过滤
	_, total, err = GetAuditEvents("", "critical", 2, 0, "", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	_, total, err = GetAuditEvents("", "critical", 1, 0, "", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)

	// 关键词过滤命中 username / rule_name / model_name
	_, total, err = GetAuditEvents("", "", 0, 0, "身份证", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	_, total, err = GetAuditEvents("", "", 0, 0, "claude", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)

	// 时间范围过滤
	_, total, err = GetAuditEvents("", "", 0, 0, "", 1500, 2500, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
}

func TestGetAuditEventStats(t *testing.T) {
	seedAuditEvents(t)

	rows, err := GetAuditEventStats(0, 0)
	require.NoError(t, err)
	counts := map[string]int64{}
	for _, r := range rows {
		counts[r.EventType+"|"+r.RuleId] = r.Count
	}
	assert.EqualValues(t, 1, counts["pii_hit|builtin.id_card_cn"])
	assert.EqualValues(t, 1, counts["pii_hit|builtin.api_key_sk"])
	assert.EqualValues(t, 1, counts["key_share_multi_ip|"])

	// 时间范围收窄后只剩一条
	rows, err = GetAuditEventStats(1500, 2500)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, AuditEventTypePiiHit, rows[0].EventType)
	assert.Equal(t, "builtin.api_key_sk", rows[0].RuleId)
}

func TestDeleteExpiredAuditEvents(t *testing.T) {
	seedAuditEvents(t)

	// 保留 1 天:now=3000+86400 时全部过期;now 取 2000+86399 时只删 created_at=1000 的
	deleted, err := DeleteExpiredAuditEvents(1, 2000+86399)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	_, total, err := GetAuditEvents("", "", 0, 0, "", 0, 0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)

	// retentionDays<=0 不删任何数据(防御误配置清空)
	deleted, err = DeleteExpiredAuditEvents(0, 99999999)
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted)
}
