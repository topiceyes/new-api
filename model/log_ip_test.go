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
