package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// RunFeishuLeaveCheck triggers one departed-employee patrol run right away
// and returns its result counts. Root only, like the patrol configuration in
// /api/option.
func RunFeishuLeaveCheck(c *gin.Context) {
	result, err := service.RunFeishuLeaveCheckNow(c.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFeishuLeaveCheckRunning):
			common.ApiErrorI18n(c, i18n.MsgFeishuPatrolAlreadyRunning)
		case errors.Is(err, service.ErrFeishuLeaveCheckNotConfigured):
			common.ApiErrorI18n(c, i18n.MsgFeishuPatrolNotConfigured)
		default:
			common.ApiErrorI18n(c, i18n.MsgFeishuPatrolFailed)
		}
		return
	}
	common.ApiSuccess(c, result)
}
