package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newChannelRateLimitContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

func newRPMChannel(id int, rpm int) *model.Channel {
	setting := "{}"
	if rpm > 0 {
		setting = `{"rate_limit_rpm":` + strconv.Itoa(rpm) + `}`
	}
	return &model.Channel{Id: id, Name: "test-channel", Setting: &setting}
}

func useChannelRateLimitMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		_ = redisClient.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})
	return redisServer
}

// 内存路径夹具:关闭 Redis。channelRateLimitMemory 不重置——Init 启动的
// 清理 goroutine 持有的是包变量本身的地址,整体替换会把运行中的互斥锁清零。
// 各测试用不同渠道 id 隔离计数。
func useChannelRateLimitMemory(t *testing.T) {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})
}

func TestCheckChannelRateLimitUnlimited(t *testing.T) {
	useChannelRateLimitMemory(t)

	tests := []struct {
		name    string
		channel *model.Channel
	}{
		{name: "nil channel", channel: nil},
		{name: "rpm unset", channel: newRPMChannel(1, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newChannelRateLimitContext()
			limited, retryAfter := CheckChannelRateLimit(c, tt.channel)
			assert.False(t, limited)
			assert.Zero(t, retryAfter)
		})
	}
}

// 内存路径:固定窗口内恰好到上限放行,上限+1 拒绝,窗口过后恢复
func TestCheckChannelRateLimitMemoryThreshold(t *testing.T) {
	useChannelRateLimitMemory(t)

	channel := newRPMChannel(11, 2)
	for i := 0; i < 2; i++ {
		c, _ := newChannelRateLimitContext()
		limited, _ := CheckChannelRateLimit(c, channel)
		require.False(t, limited, "request %d should pass", i+1)
	}

	c, _ := newChannelRateLimitContext()
	limited, retryAfter := CheckChannelRateLimit(c, channel)
	assert.True(t, limited)
	assert.Equal(t, int64(channelRateLimitWindowSeconds), retryAfter)
}

// 同一请求内对同一渠道重复检查只计一次(distributor 首次选择 + 重试重选场景)
func TestCheckChannelRateLimitDedupWithinRequest(t *testing.T) {
	useChannelRateLimitMemory(t)

	channel := newRPMChannel(12, 1)
	c, _ := newChannelRateLimitContext()

	limited, _ := CheckChannelRateLimit(c, channel)
	require.False(t, limited)

	limited, _ = CheckChannelRateLimit(c, channel)
	assert.False(t, limited, "same channel in the same request must not be counted twice")

	// 新请求仍受窗口约束
	c2, _ := newChannelRateLimitContext()
	limited, _ = CheckChannelRateLimit(c2, channel)
	assert.True(t, limited)
}

// Redis 路径:INCR 计数、窗口 TTL 作为 Retry-After、FastForward 后恢复
func TestCheckChannelRateLimitRedisThreshold(t *testing.T) {
	redisServer := useChannelRateLimitMiniRedis(t)

	channel := newRPMChannel(13, 2)
	for i := 0; i < 2; i++ {
		c, _ := newChannelRateLimitContext()
		limited, _ := CheckChannelRateLimit(c, channel)
		require.False(t, limited, "request %d should pass", i+1)
	}

	c, _ := newChannelRateLimitContext()
	limited, retryAfter := CheckChannelRateLimit(c, channel)
	assert.True(t, limited)
	assert.Greater(t, retryAfter, int64(0))
	assert.LessOrEqual(t, retryAfter, int64(channelRateLimitWindowSeconds))

	count, err := redisServer.Get(channelRateLimitRedisKey(channel.Id))
	require.NoError(t, err)
	assert.Equal(t, "3", count)
	assert.Equal(t, channelRateLimitWindowSeconds*time.Second, redisServer.TTL(channelRateLimitRedisKey(channel.Id)))

	redisServer.FastForward(channelRateLimitWindowSeconds*time.Second + time.Second)
	c2, _ := newChannelRateLimitContext()
	limited, _ = CheckChannelRateLimit(c2, channel)
	assert.False(t, limited, "window expiry should reset the limit")
}

// Redis 故障时放行(fail-open),不阻断用户流量
func TestCheckChannelRateLimitRedisErrorFailsOpen(t *testing.T) {
	useChannelRateLimitMiniRedis(t)
	require.NoError(t, common.RDB.Close())

	c, _ := newChannelRateLimitContext()
	limited, _ := CheckChannelRateLimit(c, newRPMChannel(14, 1))
	assert.False(t, limited)
}

func TestNewChannelRateLimitError(t *testing.T) {
	require.NoError(t, i18n.Init())

	channel := newRPMChannel(15, 5)
	c, recorder := newChannelRateLimitContext()

	apiErr := NewChannelRateLimitError(c, channel, 42)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeChannelRateLimited, apiErr.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.Equal(t, "42", recorder.Header().Get("Retry-After"))
	assert.Contains(t, apiErr.Error(), channel.Name)
}
