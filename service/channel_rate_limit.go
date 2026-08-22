package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// channelRateLimitWindowSeconds 是单渠道 RPM 限速的固定窗口长度。
// 固定窗口对小 RPM 值语义最直观:窗口内打满即拒,下一分钟恢复,
// 且窗口 TTL 可直接作为 Retry-After。
const channelRateLimitWindowSeconds = 60

// contextChannelRateCounted 记录本次请求已经消耗过限速配额的渠道 id,
// 避免同一请求内(distributor 首次选择 + getChannel 重试重选)对同一渠道重复计数。
const contextChannelRateCounted = "channel_rl_counted"

// 与 middleware/rate-limit.go 的 redisFixedWindowScript 同一固定窗口脚本:
// INCR + EXPIRE 原子化,窗口边界处流量最多突发到两倍限额。
// service 包不能 import middleware(distributor 反向依赖),故在此保留一份。
const channelRateLimitRedisScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  ttl = redis.call('TTL', KEYS[1])
end
if count > tonumber(ARGV[1]) then
  return {0, count, ttl}
end
return {1, count, ttl}
`

var channelRateLimitMemory common.InMemoryRateLimiter

func channelRateLimitRedisKey(channelId int) string {
	return fmt.Sprintf("rateLimit:v2:channel:%d", channelId)
}

// CheckChannelRateLimit 检查渠道 RPM 限速并在未超限时消耗一次配额。
// 返回 limited=true 表示超限,调用方应拒绝请求;retryAfterSeconds 仅在内存
// 路径无法精确时为窗口长度的保守上限。
// Redis 故障时放行并记录错误(限速是保护手段,不应因 Redis 抖动阻断全部流量)。
func CheckChannelRateLimit(c *gin.Context, channel *model.Channel) (limited bool, retryAfterSeconds int64) {
	if channel == nil {
		return false, 0
	}
	rpm := channel.GetSetting().RateLimitRPM
	if rpm <= 0 {
		return false, 0
	}

	channelIdStr := strconv.Itoa(channel.Id)
	for _, id := range c.GetStringSlice(contextChannelRateCounted) {
		if id == channelIdStr {
			return false, 0
		}
	}

	key := channelRateLimitRedisKey(channel.Id)
	var allowed bool
	if common.RedisEnabled && common.RDB != nil {
		var err error
		allowed, retryAfterSeconds, err = channelRateLimitRedisTake(c.Request.Context(), key, rpm)
		if err != nil {
			common.SysError(fmt.Sprintf("channel rate limit check failed (channel #%d): %v", channel.Id, err))
			return false, 0
		}
	} else {
		channelRateLimitMemory.Init(common.RateLimitKeyExpirationDuration)
		allowed = channelRateLimitMemory.Request(key, rpm, channelRateLimitWindowSeconds)
		retryAfterSeconds = channelRateLimitWindowSeconds
	}
	if !allowed {
		return true, retryAfterSeconds
	}

	counted := append(c.GetStringSlice(contextChannelRateCounted), channelIdStr)
	c.Set(contextChannelRateCounted, counted)
	return false, 0
}

func channelRateLimitRedisTake(ctx context.Context, key string, rpm int) (bool, int64, error) {
	values, err := common.RDB.Eval(
		ctx,
		channelRateLimitRedisScript,
		[]string{key},
		rpm,
		channelRateLimitWindowSeconds,
	).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(values) != 3 {
		return false, 0, fmt.Errorf("unexpected channel rate limit reply length %d", len(values))
	}
	allowedValue, err := redisReplyInteger(values[0])
	if err != nil {
		return false, 0, err
	}
	ttlSeconds, err := redisReplyInteger(values[2])
	if err != nil {
		return false, 0, err
	}
	return allowedValue == 1, ttlSeconds, nil
}

func redisReplyInteger(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer reply type %T", value)
	}
}

// NewChannelRateLimitError 构造渠道限速 429 错误:带 Retry-After 头,
// 标记 skip-retry,保证重试逻辑不会改投其他渠道、用户能看到提示。
func NewChannelRateLimitError(c *gin.Context, channel *model.Channel, retryAfterSeconds int64) *types.NewAPIError {
	if retryAfterSeconds > 0 {
		c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	}
	msg := i18n.T(c, i18n.MsgRelayChannelRateLimited, map[string]any{
		"Channel": channel.Name,
		"RPM":     channel.GetSetting().RateLimitRPM,
	})
	return types.NewErrorWithStatusCode(
		errors.New(msg),
		types.ErrorCodeChannelRateLimited,
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
	)
}
