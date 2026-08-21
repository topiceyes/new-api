package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedPlanMonitor(t *testing.T, provider string, enabled, isPublic bool, sortOrder int) *PlanMonitor {
	t.Helper()
	p := &PlanMonitor{
		Provider:  provider,
		PlanName:  provider + "-plan",
		ApiKey:    "sk-test",
		Enabled:   enabled,
		IsPublic:  isPublic,
		SortOrder: sortOrder,
	}
	require.NoError(t, CreatePlanMonitor(p))
	return p
}

// 公开套餐列表只含「启用且公开」的套餐,排序与管理员列表一致(sort_order→provider→id)。
func TestGetPublicPlanMonitorsFiltersAndOrders(t *testing.T) {
	truncateTables(t)

	publicEnabled2 := seedPlanMonitor(t, "kimi", true, true, 2)
	publicEnabled1 := seedPlanMonitor(t, "minimax", true, true, 1)
	seedPlanMonitor(t, "bigmodel", false, true, 0)  // 公开但停用
	seedPlanMonitor(t, "volcengine", true, false, 0) // 启用但未公开
	seedPlanMonitor(t, "opencode", false, false, 0)  // 都否

	plans, err := GetPublicPlanMonitors()
	require.NoError(t, err)
	require.Len(t, plans, 2)
	assert.Equal(t, publicEnabled1.Id, plans[0].Id)
	assert.Equal(t, publicEnabled2.Id, plans[1].Id)
}

// UpdatePlanMonitor 的 map 必须能写 is_public 的零值 false,否则开关关不掉。
func TestUpdatePlanMonitorIsPublicZeroValuePersists(t *testing.T) {
	truncateTables(t)

	p := seedPlanMonitor(t, "kimi", true, true, 0)
	p.IsPublic = false
	require.NoError(t, UpdatePlanMonitor(p))

	reloaded, err := GetPlanMonitorById(p.Id)
	require.NoError(t, err)
	assert.False(t, reloaded.IsPublic, "true→false 的 is_public 更新必须落库")
}
