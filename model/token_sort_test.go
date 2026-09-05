package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 令牌列表排序: used_tokens 走 LOG_DB 聚合的 Go 侧排序(跨库不能 JOIN),
// accessed_time 走 SQL;非法 sort_by 回退 id。
func TestTokenSortByUsedTokens(t *testing.T) {
	truncateTables(t)

	mk := func(id int, name string) {
		require.NoError(t, DB.Create(&Token{UserId: 1, Key: "sk-sort-" + name, Name: name}).Error)
	}
	mk(0, "a")
	mk(0, "b")
	mk(0, "c")
	var tokens []Token
	require.NoError(t, DB.Order("id").Find(&tokens).Error)
	require.Len(t, tokens, 3)

	// a: 300 tokens, c: 100 tokens, b: 无日志(0)
	logs := []Log{
		{UserId: 1, TokenId: tokens[0].Id, Type: LogTypeConsume, CreatedAt: 1, PromptTokens: 200, CompletionTokens: 100},
		{UserId: 1, TokenId: tokens[2].Id, Type: LogTypeConsume, CreatedAt: 1, PromptTokens: 100},
	}
	for i := range logs {
		require.NoError(t, DB.Create(&logs[i]).Error)
	}

	got, err := GetAllUserTokens(1, 0, 10, NewTokenSortOptions("used_tokens", "desc"))
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "a", got[0].Name) // 300
	assert.Equal(t, "c", got[1].Name) // 100
	assert.Equal(t, "b", got[2].Name) // 0

	got, err = GetAllUserTokens(1, 0, 10, NewTokenSortOptions("used_tokens", "asc"))
	require.NoError(t, err)
	assert.Equal(t, "b", got[0].Name)
	assert.Equal(t, "a", got[2].Name)

	// 分页切片
	got, err = GetAllUserTokens(1, 1, 1, NewTokenSortOptions("used_tokens", "desc"))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "c", got[0].Name)
}

func TestNewTokenSortOptions(t *testing.T) {
	assert.Equal(t, TokenSortOptions{SortBy: "accessed_time", SortOrder: "asc"}, NewTokenSortOptions("accessed_time", "asc"))
	assert.Equal(t, TokenSortOptions{SortBy: "used_tokens", SortOrder: "desc"}, NewTokenSortOptions("used_tokens", ""))
	// 非法值回退 id desc(防注入)
	assert.Equal(t, TokenSortOptions{SortBy: "id", SortOrder: "desc"}, NewTokenSortOptions("id; DROP TABLE tokens", "desc"))
	assert.Equal(t, TokenSortOptions{SortBy: "id", SortOrder: "desc"}, NewTokenSortOptions("", ""))
}
