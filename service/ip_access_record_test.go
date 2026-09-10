package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
)

func resetRateLimitedIPMemory(t *testing.T) {
	t.Helper()
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	rateLimitedIPMemory.Lock()
	rateLimitedIPMemory.records = map[string]*RateLimitedIPRecord{}
	rateLimitedIPMemory.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		rateLimitedIPMemory.Lock()
		rateLimitedIPMemory.records = map[string]*RateLimitedIPRecord{}
		rateLimitedIPMemory.Unlock()
	})
}

func stubRateLimitedIPNow(t *testing.T, now *int64) {
	t.Helper()
	old := rateLimitedIPNow
	rateLimitedIPNow = func() int64 { return *now }
	t.Cleanup(func() { rateLimitedIPNow = old })
}

func TestRecordRateLimitedIPAccumulates(t *testing.T) {
	resetRateLimitedIPMemory(t)

	now := int64(1700000000)
	stubRateLimitedIPNow(t, &now)

	RecordRateLimitedIP("203.0.113.10", "CT")
	RecordRateLimitedIP("203.0.113.10", "CT")
	now += 60
	RecordRateLimitedIP("203.0.113.10", "GW")

	records, err := ListRateLimitedIPs()
	require.NoError(t, err)
	require.Len(t, records, 1)
	rec := records[0]
	assert.Equal(t, "203.0.113.10", rec.IP)
	assert.Equal(t, int64(3), rec.Hits)
	assert.Equal(t, []string{"CT", "GW"}, rec.Marks) // 同桶去重
	assert.Equal(t, int64(1700000000), rec.FirstSeen)
	assert.Equal(t, int64(1700000060), rec.LastSeen)
}

func TestRecordRateLimitedIPIgnoresEmptyIP(t *testing.T) {
	resetRateLimitedIPMemory(t)
	RecordRateLimitedIP("", "CT")
	records, err := ListRateLimitedIPs()
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestListRateLimitedIPsSortedByLastSeenDesc(t *testing.T) {
	resetRateLimitedIPMemory(t)

	now := int64(1700000000)
	stubRateLimitedIPNow(t, &now)

	RecordRateLimitedIP("203.0.113.1", "CT")
	now += 10
	RecordRateLimitedIP("203.0.113.2", "CT")
	now += 10
	RecordRateLimitedIP("203.0.113.3", "CT")

	records, err := ListRateLimitedIPs()
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, "203.0.113.3", records[0].IP)
	assert.Equal(t, "203.0.113.2", records[1].IP)
	assert.Equal(t, "203.0.113.1", records[2].IP)
}

func TestDeleteRateLimitedIPRecord(t *testing.T) {
	resetRateLimitedIPMemory(t)
	RecordRateLimitedIP("203.0.113.10", "CT")
	RecordRateLimitedIP("203.0.113.11", "CT")

	require.NoError(t, DeleteRateLimitedIPRecord("203.0.113.10"))
	// 删除不存在的记录不报错
	require.NoError(t, DeleteRateLimitedIPRecord("203.0.113.10"))

	records, err := ListRateLimitedIPs()
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "203.0.113.11", records[0].IP)
}
