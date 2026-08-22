package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				common.SysLog(fmt.Sprintf("panic detected: %v", err))
				common.SysLog(fmt.Sprintf("stacktrace from panic: %s", string(debug.Stack())))
				// Neutral wire response: panic details stay in the server logs
				// above; the client gets no internal error text or project links.
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"message": "Internal server error.",
						"type":    "internal_error",
					},
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
