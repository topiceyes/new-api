package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

// userRateLimitWindowSeconds 用户级 RPM 限速的固定窗口长度,与渠道 RPM 一致:
// 窗口内打满即拒,下一分钟恢复,窗口 TTL 直接作为 Retry-After。
const userRateLimitWindowSeconds = 60

// UserRequestRateLimit enforces the per-user RPM configured by admins in the
// user's setting JSON (rate_limit_rpm). It must run AFTER TokenAuth, which
// loads the parsed dto.UserSetting into the request context, so the check
// costs no extra DB/Redis read. All tokens of a user share one counter.
// Redis failures fail open (logged): rate limiting is a protective measure and
// must not block all traffic on a Redis blip.
func UserRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		userSetting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
		if !ok || userSetting.RateLimitRPM <= 0 {
			c.Next()
			return
		}
		userID := c.GetInt("id")
		if userID == 0 {
			c.Next()
			return
		}
		rpm := userSetting.RateLimitRPM

		var allowed bool
		var retryAfterSeconds int64
		if common.RedisEnabled && common.RDB != nil {
			var err error
			allowed, _, retryAfterSeconds, err = redisFixedWindowTake(
				c.Request.Context(),
				redisUserRateLimitKey("URPM", userID),
				rpm,
				userRateLimitWindowSeconds,
			)
			if err != nil {
				common.SysError(fmt.Sprintf("user rate limit check failed (user #%d): %v", userID, err))
				c.Next()
				return
			}
		} else {
			inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
			key := fmt.Sprintf("URPM:user:%d", userID)
			allowed = inMemoryRateLimiter.Request(key, rpm, userRateLimitWindowSeconds)
			retryAfterSeconds = userRateLimitWindowSeconds
		}

		if allowed {
			c.Next()
			return
		}
		if retryAfterSeconds > 0 {
			c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
		}
		abortWithOpenAiMessage(c, http.StatusTooManyRequests,
			i18n.T(c, i18n.MsgRelayUserRateLimited, map[string]any{"RPM": rpm}))
	}
}
