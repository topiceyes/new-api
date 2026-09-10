package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// IPAccessControl 黑名单全站拦截:命中黑名单的 IP 直接 403,不进入任何
// 业务逻辑(包括 relay)。白名单不在这里处理——白名单只跳过按 IP 的限流,
// 见 rate-limit.go 的 rateLimitFactory。
func IPAccessControl() func(c *gin.Context) {
	return func(c *gin.Context) {
		if system_setting.IsIPBlacklisted(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Your IP has been blocked by the administrator.",
			})
			return
		}
		c.Next()
	}
}
