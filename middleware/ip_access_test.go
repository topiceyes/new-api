package middleware

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withIPAccessLists(t *testing.T, whitelist, blacklist []string) {
	t.Helper()
	settings := system_setting.GetIPAccessSettings()
	oldWhitelist := settings.Whitelist
	oldBlacklist := settings.Blacklist
	settings.Whitelist = whitelist
	settings.Blacklist = blacklist
	t.Cleanup(func() {
		settings.Whitelist = oldWhitelist
		settings.Blacklist = oldBlacklist
	})
}

func TestIPAccessControlBlocksBlacklistedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withIPAccessLists(t, nil, []string{"192.0.2.55", "198.51.100.0/24"})

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(IPAccessControl())
	router.GET("/any", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	blocked := performRateLimitRequest(router, "/any", "192.0.2.55:12345")
	assert.Equal(t, http.StatusForbidden, blocked.Code)

	blockedCIDR := performRateLimitRequest(router, "/any", "198.51.100.7:12345")
	assert.Equal(t, http.StatusForbidden, blockedCIDR.Code)

	allowed := performRateLimitRequest(router, "/any", "203.0.113.9:12345")
	assert.Equal(t, http.StatusNoContent, allowed.Code)
}

func TestRateLimitFactorySkipsWhitelistedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	withIPAccessLists(t, []string{"192.0.2.10"}, nil)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/limited", rateLimitFactory(1, 60, "TESTWL"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// 白名单 IP 反复请求不触发限流
	for i := 0; i < 5; i++ {
		resp := performRateLimitRequest(router, "/limited", "192.0.2.10:12345")
		assert.Equal(t, http.StatusNoContent, resp.Code, "request %d", i)
	}

	// 非白名单 IP 第二次请求被限流
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "203.0.113.9:12345").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "203.0.113.9:12345").Code)
}

func TestRateLimitedIPRecordedOnRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	withIPAccessLists(t, nil, nil)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/limited", rateLimitFactory(1, 60, "CT"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.77:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", remoteAddr).Code)

	records, err := service.ListRateLimitedIPs()
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "192.0.2.77", records[0].IP)
	assert.Equal(t, []string{"CT"}, records[0].Marks)
	assert.Equal(t, int64(1), records[0].Hits)
}
