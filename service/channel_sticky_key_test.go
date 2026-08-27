package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 内存路径单测:粘性绑定的独占/复用/释放/换绑语义。Redis 路径与内存路径共用
// 同一套语义,由 E2E 覆盖。
// 渠道 id 使用 87000+ 避开其他测试在 stickyMem 里的状态。

func withStickyClock(t *testing.T, start int64) func(advance int64) {
	t.Helper()
	now := start
	stickyClockNow = func() time.Time { return time.Unix(now, 0) }
	t.Cleanup(func() { stickyClockNow = time.Now })
	return func(advance int64) { now += advance }
}

func TestBindTokenMemoryExclusiveAndReuse(t *testing.T) {
	withStickyClock(t, 1000)
	enabled := []int{0, 1}

	idx, ok := bindTokenMemory(87001, 87001, enabled, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 0, idx)

	idx2, ok := bindTokenMemory(87002, 87001, enabled, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 1, idx2)

	// 已绑定的令牌复用自己的 key。
	idx, ok = bindTokenMemory(87001, 87001, enabled, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 0, idx)

	// 全部占满:busy 拒绝,share 随机共用且粘住。
	_, ok = bindTokenMemory(87003, 87001, enabled, 10*time.Minute, false)
	assert.False(t, ok)
	shared, ok := bindTokenMemory(87003, 87001, enabled, 10*time.Minute, true)
	require.True(t, ok)
	assert.Contains(t, enabled, shared)
	again, ok := bindTokenMemory(87003, 87001, enabled, 10*time.Minute, true)
	require.True(t, ok)
	assert.Equal(t, shared, again, "share 绑定也应粘性复用")

	// 列表:两个独占 + 一个共享。
	list := listStickyMemory(87001, 10*time.Minute)
	require.Len(t, list, 3)
	byToken := map[int]StickyBinding{}
	for _, b := range list {
		byToken[b.TokenId] = b
	}
	assert.True(t, byToken[87001].Exclusive)
	assert.True(t, byToken[87002].Exclusive)
	assert.False(t, byToken[87003].Exclusive, "share 绑定不是独占")
	assert.Equal(t, 0, byToken[87001].IdleSeconds)

	// 解绑独占者后,key 0 可被新令牌绑定。
	releaseStickyMemory(87001, 87001)
	idx, ok = bindTokenMemory(87004, 87001, enabled, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 0, idx)
}

func TestBindTokenMemoryIdleExpiry(t *testing.T) {
	advance := withStickyClock(t, 2000)
	enabled := []int{0}

	idx, ok := bindTokenMemory(87002, 87002, enabled, 5*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 0, idx)

	// 活跃刷新:每次访问推后释放。
	advance(240)
	_, ok = bindTokenMemory(87002, 87002, enabled, 5*time.Minute, false)
	require.True(t, ok)
	advance(240) // 距上次活跃 4 分钟,仍未过期
	_, ok = bindTokenMemory(87002, 87002, enabled, 5*time.Minute, false)
	require.True(t, ok)

	advance(301) // 距上次活跃 > 5 分钟,绑定释放
	assert.Empty(t, listStickyMemory(87002, 5*time.Minute))

	// 释放后的 key 可被其他令牌绑定。
	idx, ok = bindTokenMemory(87005, 87002, enabled, 5*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 0, idx)
}

func TestBindTokenMemoryDisabledKeyRebind(t *testing.T) {
	withStickyClock(t, 3000)

	// 令牌绑定 key 0,之后 key 0 被禁用 -> 自动换绑到 key 1。
	idx, ok := bindTokenMemory(87003, 87003, []int{0, 1}, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 0, idx)

	idx, ok = bindTokenMemory(87003, 87003, []int{1}, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 1, idx)

	// 越界(管理员删了 key)同样换绑。
	idx, ok = bindTokenMemory(87003, 87003, []int{2, 3}, 10*time.Minute, false)
	require.True(t, ok)
	assert.Equal(t, 2, idx)
}

func TestBindTokenMemoryEmptyEnabled(t *testing.T) {
	withStickyClock(t, 4000)
	// 候选为空:busy 拒绝;share 不能在空集上随机(直接拒绝,由调用方兜底)。
	_, ok := bindTokenMemory(87004, 87004, nil, 10*time.Minute, false)
	assert.False(t, ok)
	_, ok = bindTokenMemory(87004, 87004, nil, 10*time.Minute, true)
	assert.False(t, ok)
}

func TestStickyIdleDurationDefault(t *testing.T) {
	assert.Equal(t, 10*time.Minute, stickyIdleDuration(0))
	assert.Equal(t, 10*time.Minute, stickyIdleDuration(-1))
	assert.Equal(t, 3*time.Minute, stickyIdleDuration(3))
}
