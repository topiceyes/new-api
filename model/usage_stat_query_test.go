package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 用户分析表格的按用户聚合: active_days 按 request_count>0 的天去重,
// 纯失败天不算活跃; last_active_date 取最大活跃日,全失败用户回退空串(COALESCE)。
func TestQueryUsageUserTable(t *testing.T) {
	truncateTables(t)

	rows := []UsageStatDaily{
		// 用户1: 三天里有两天有 consume(d1/d3),d2 只有失败 -> active_days=2
		{Date: "2026-08-01", UserId: 1, Username: "u1", ModelName: "m1", RequestCount: 2, Quota: 100, TotalUseTime: 10},
		{Date: "2026-08-02", UserId: 1, Username: "u1", ModelName: "m1", FailCount: 1},
		{Date: "2026-08-03", UserId: 1, Username: "u1", ModelName: "m2", RequestCount: 1, FailCount: 2, Quota: 50, RefundQuota: 20, TotalUseTime: 5},
		// 用户2: 范围内全是失败日志,无 consume -> active_days=0, last_active_date=''
		{Date: "2026-08-01", UserId: 2, Username: "u2", ModelName: "m1", FailCount: 3},
		// 范围外数据不进入统计
		{Date: "2026-07-01", UserId: 1, Username: "u1", ModelName: "m1", RequestCount: 9, Quota: 999},
	}
	require.NoError(t, DB.CreateInBatches(rows, 100).Error)

	got, err := QueryUsageUserTable("2026-08-01", "2026-08-03", nil)
	require.NoError(t, err)
	require.Len(t, got, 2)

	byUser := map[int]UsageUserTableRow{}
	for _, r := range got {
		byUser[r.UserId] = r
	}

	u1 := byUser[1]
	assert.Equal(t, "u1", u1.Username)
	assert.Equal(t, int64(3), u1.RequestCount)
	assert.Equal(t, int64(3), u1.FailCount)
	assert.Equal(t, int64(150), u1.Quota)
	assert.Equal(t, int64(20), u1.RefundQuota)
	assert.Equal(t, int64(15), u1.TotalUseTime)
	assert.Equal(t, int64(2), u1.ActiveDays)
	assert.Equal(t, "2026-08-03", u1.LastActiveDate)

	u2 := byUser[2]
	assert.Equal(t, int64(0), u2.RequestCount)
	assert.Equal(t, int64(3), u2.FailCount)
	assert.Equal(t, int64(0), u2.ActiveDays)
	assert.Equal(t, "", u2.LastActiveDate)
}

// 用户x模型聚合: 同一用户不同模型拆成多行,同名模型跨天合并。
func TestQueryUsageByUserModel(t *testing.T) {
	truncateTables(t)

	rows := []UsageStatDaily{
		{Date: "2026-08-01", UserId: 1, Username: "u1", ModelName: "m1", RequestCount: 2, Quota: 100},
		{Date: "2026-08-02", UserId: 1, Username: "u1", ModelName: "m1", RequestCount: 1, Quota: 50},
		{Date: "2026-08-02", UserId: 1, Username: "u1", ModelName: "m2", RequestCount: 5, Quota: 10},
		{Date: "2026-08-01", UserId: 2, Username: "u2", ModelName: "m1", RequestCount: 7, Quota: 70},
	}
	require.NoError(t, DB.CreateInBatches(rows, 100).Error)

	got, err := QueryUsageByUserModel("2026-08-01", "2026-08-03", nil)
	require.NoError(t, err)
	require.Len(t, got, 3)

	type key struct {
		user  int
		model string
	}
	byKey := map[key]UsageUserModelRow{}
	for _, r := range got {
		byKey[key{r.UserId, r.ModelName}] = r
	}
	assert.Equal(t, int64(3), byKey[key{1, "m1"}].RequestCount)
	assert.Equal(t, int64(150), byKey[key{1, "m1"}].Quota)
	assert.Equal(t, int64(5), byKey[key{1, "m2"}].RequestCount)
	assert.Equal(t, int64(7), byKey[key{2, "m1"}].RequestCount)
}

// 无 org 快照时的兜底全集: 只取三列,软删用户不出现。
func TestGetUserIdentities(t *testing.T) {
	truncateTables(t)

	// aff_code 有唯一索引,批量造用户必须各不相同。
	require.NoError(t, DB.Create(&User{Username: "alice", DisplayName: "Alice", Password: "x", AffCode: "aff-a"}).Error)
	require.NoError(t, DB.Create(&User{Username: "bob", Password: "x", AffCode: "aff-b"}).Error)
	deleted := &User{Username: "carol", Password: "x", AffCode: "aff-c"}
	require.NoError(t, DB.Create(deleted).Error)
	require.NoError(t, DB.Delete(deleted).Error)

	got, err := GetUserIdentities()
	require.NoError(t, err)
	require.Len(t, got, 2)

	names := map[string]string{}
	for _, u := range got {
		names[u.Username] = u.DisplayName
		assert.Empty(t, u.Password)
	}
	assert.Equal(t, "Alice", names["alice"])
	assert.Contains(t, names, "bob")
}
