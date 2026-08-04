package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// RunDingTalkLeaveCheck triggers one departed-employee patrol run right away
// and returns its result counts. Root only, like the patrol configuration in
// /api/option.
func RunDingTalkLeaveCheck(c *gin.Context) {
	result, err := service.RunDingTalkLeaveCheckNow(c.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDingTalkLeaveCheckRunning):
			common.ApiErrorI18n(c, i18n.MsgDingTalkPatrolAlreadyRunning)
		case errors.Is(err, service.ErrDingTalkLeaveCheckNotConfigured):
			common.ApiErrorI18n(c, i18n.MsgDingTalkPatrolNotConfigured)
		default:
			common.ApiErrorI18n(c, i18n.MsgDingTalkPatrolFailed)
		}
		return
	}
	common.ApiSuccess(c, result)
}
