package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 每用户最近带 IP 日志: 取 MAX(id) 的那条;无 IP 日志/空入参不出现在结果里。
func TestGetLatestLogIpByUsers(t *testing.T) {
	truncateTables(t)

	logs := []Log{
		// 用户1: 旧日志 ip=a, 新日志 ip=b -> b
		{UserId: 1, Username: "u1", Type: LogTypeConsume, Ip: "1.1.1.1"},
		{UserId: 1, Username: "u1", Type: LogTypeLogin, Ip: "2.2.2.2"},
		// 用户2: 只有 ip 为空的日志 -> 不出现
		{UserId: 2, Username: "u2", Type: LogTypeConsume, Ip: ""},
		// 用户3: 完全没有日志 -> 不出现
	}
	for i := range logs {
		require.NoError(t, DB.Create(&logs[i]).Error)
	}

	got, err := GetLatestLogIpByUsers([]int{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, map[int]string{1: "2.2.2.2"}, got)

	empty, err := GetLatestLogIpByUsers(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// key 管理页用量摘要: 累计 tokens 只计 consume;最近 IP/UA 取 created_at 最大的
// 那条 consume 日志(空 IP 也照样算"最近",与列表展示一致)。
func TestGetTokenUsageSummaries(t *testing.T) {
	truncateTables(t)

	logs := []Log{
		{UserId: 1, TokenId: 10, Type: LogTypeConsume, CreatedAt: 100, PromptTokens: 100, CompletionTokens: 50, Ip: "1.1.1.1", Other: `{"user_agent":"curl/8.0"}`},
		{UserId: 1, TokenId: 10, Type: LogTypeConsume, CreatedAt: 200, PromptTokens: 300, CompletionTokens: 100, Ip: "2.2.2.2", Other: `{"user_agent":"python-requests/2.31"}`},
		// 非 consume 类型不参与统计
		{UserId: 1, TokenId: 10, Type: LogTypeManage, CreatedAt: 300, Ip: "9.9.9.9"},
		// 无 UA 的旧日志: UA 留空但 IP 仍取最近行
		{UserId: 1, TokenId: 11, Type: LogTypeConsume, CreatedAt: 150, PromptTokens: 7, Ip: "3.3.3.3", Other: `{}`},
		// 用户没开「记录 IP」: ip 列为空,审计 IP 在 admin_info 里(root 视图用)
		{UserId: 1, TokenId: 13, Type: LogTypeConsume, CreatedAt: 160, PromptTokens: 5, Ip: "", Other: `{"admin_info":{"admin_ip":"198.51.100.7"}}`},
	}
	for i := range logs {
		require.NoError(t, DB.Create(&logs[i]).Error)
	}

	got, err := GetTokenUsageSummaries([]int{10, 11, 12, 13})
	require.NoError(t, err)
	require.Len(t, got, 3) // token 12 无日志

	s10 := got[10]
	assert.Equal(t, int64(550), s10.TotalTokens)
	assert.Equal(t, "2.2.2.2", s10.LastIp)
	assert.Equal(t, "python-requests/2.31", s10.LastUserAgent)

	s11 := got[11]
	assert.Equal(t, int64(7), s11.TotalTokens)
	assert.Equal(t, "3.3.3.3", s11.LastIp)
	assert.Equal(t, "", s11.LastUserAgent)

	s13 := got[13]
	assert.Equal(t, "", s13.LastIp) // 开关未开,ip 列为空
	assert.Equal(t, "198.51.100.7", s13.AdminLastIp)

	empty, err := GetTokenUsageSummaries(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// 日志搜索的用户名解析: 输入同时匹配 username 与显示名;无命中时输入本身作
// 候选(兜底已删除用户的历史日志);空输入=不过滤(nil)。
func TestResolveLogSearchUsernames(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&User{Username: "zhangsan", DisplayName: "张三", Password: "x", AffCode: "aff-zs"}).Error)
	require.NoError(t, DB.Create(&User{Username: "lisi", DisplayName: "李四", Password: "x", AffCode: "aff-ls"}).Error)
	require.NoError(t, DB.Create(&User{Username: "zhangwu", DisplayName: "张五", Password: "x", AffCode: "aff-zw"}).Error)

	// 按显示名精确命中: 命中账号 username + 输入本身兜底候选
	got, err := ResolveLogSearchUsernames("张三")
	require.NoError(t, err)
	assert.Equal(t, []string{"zhangsan", "张三"}, got)

	// 通配符同时命中 username 前缀与显示名(通配输入不追加自身)
	got, err = ResolveLogSearchUsernames("%张%")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"zhangsan", "zhangwu"}, got)

	// 无命中: 输入本身兜底(已删除用户的历史日志仍可按 username 精确查)
	got, err = ResolveLogSearchUsernames("deleted-user")
	require.NoError(t, err)
	assert.Equal(t, []string{"deleted-user"}, got)

	// 空输入: nil = 不过滤
	got, err = ResolveLogSearchUsernames("")
	require.NoError(t, err)
	assert.Nil(t, got)

	// 非法通配模式直接报错
	_, err = ResolveLogSearchUsernames("%%")
	assert.Error(t, err)
}
