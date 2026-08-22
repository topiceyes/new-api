package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetOrgOverview 返回当前启用 provider 的组织架构快照(部门+成员),
// 供管理端「组织架构」页组树展示。无快照时返回空列表而非报错,
// 前端据此显示「尚未同步」空态。
func GetOrgOverview(c *gin.Context) {
	provider := service.ActiveOrgSyncProvider()
	if provider == "" {
		common.ApiErrorMsg(c, "尚未启用钉钉或飞书企业登录，组织架构同步不可用")
		return
	}
	depts, err := model.GetOrgDepartments(provider)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	members, err := model.GetOrgMembers(provider)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var syncedAt int64
	if len(depts) > 0 {
		syncedAt = depts[0].SyncedAt
	}
	common.ApiSuccess(c, gin.H{
		"provider":    provider,
		"departments": depts,
		"members":     members,
		"synced_at":   syncedAt,
	})
}

// GetOrgSyncStatus 返回最近一次 org_sync 系统任务的状态,前端轮询它感知
// 手动/定时同步的进度与结果。
func GetOrgSyncStatus(c *gin.Context) {
	latest, err := model.GetLatestSystemTasks([]string{model.SystemTaskTypeOrgSync})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	task := latest[model.SystemTaskTypeOrgSync]
	if task == nil {
		common.ApiSuccess(c, gin.H{"task": nil})
		return
	}
	resp := task.ToResponse()
	common.ApiSuccess(c, gin.H{"task": resp})
}

// RunOrgSync 手动触发一次组织架构同步:入队 org_sync 系统任务,由 task
// runner 异步执行(与定时调度共用 DB 租约,不会并发重入)。已存在进行中的
// 任务时返回 already_running=true,前端轮询 status 端点等它完成即可。
func RunOrgSync(c *gin.Context) {
	if service.ActiveOrgSyncProvider() == "" {
		common.ApiErrorMsg(c, "尚未启用钉钉或飞书企业登录，组织架构同步不可用")
		return
	}
	_, created, err := service.EnqueueSystemTask(model.SystemTaskTypeOrgSync, nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		common.ApiErrorMsg(c, "已有同步任务正在进行中")
		return
	}
	common.ApiSuccess(c, gin.H{"queued": true})
}
