package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// AdminGetRateLimitedIPs 返回全部触发过限流(429)的 IP 记录,按最近触发倒序。
// 管理员在 IP 访问控制区块里查看并一键加白/加黑。
func AdminGetRateLimitedIPs(c *gin.Context) {
	records, err := service.ListRateLimitedIPs()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取限流记录失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    records,
	})
}

// AdminDeleteRateLimitedIP 删除一条限流触发记录(不影响该 IP 的实际限流计数)。
func AdminDeleteRateLimitedIP(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "缺少 IP 参数",
		})
		return
	}
	if err := service.DeleteRateLimitedIPRecord(ip); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "删除失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
