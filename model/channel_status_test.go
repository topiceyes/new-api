package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]interface{}{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

// 多 Key 渠道在整体状态已是 Enabled(部分 key 被禁用)时,UpdateChannelStatus
// 请求 Enabled 不能提前返回——必须进入 handlerMultiKeyUpdate 按 usingKey 清单个
// key 的禁用状态,否则 key 级自动恢复永远落不了库(缓存侧却会清,造成双写不一致)。
func TestUpdateChannelStatusReEnablesMultiKeyOnEnabledChannel(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-reenable",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	// 渠道仍是 Enabled,此时请求 Enabled 恢复 key-a:修复前会提前 return false。
	changed = UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, "")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
	assert.Empty(t, stored.ChannelInfo.MultiKeyDisabledReason)
	assert.Empty(t, stored.ChannelInfo.MultiKeyDisabledTime)
}

// 全部 key 被禁用后渠道整体 AutoDisabled;恢复其中一个 key 应把渠道抬回 Enabled。
func TestUpdateChannelStatusReEnableSingleKeyLiftsChannelBack(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-lift",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "err"))
	require.True(t, UpdateChannelStatus(channel.Id, "key-b", common.ChannelStatusAutoDisabled, "err"))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, ""))

	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Len(t, stored.ChannelInfo.MultiKeyStatusList, 1)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[1])
}

func TestGetDisabledKeyIndexes(t *testing.T) {
	nonMulti := &Channel{Key: "single"}
	assert.Nil(t, nonMulti.GetAutoDisabledKeyIndexes())

	// 只有自动禁用(3)会被探测恢复;手动禁用(2)不在其列,不能违背管理员意图。
	channel := &Channel{
		Key: "k0\nk1\nk2\nk3",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 4,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusAutoDisabled,
				3: common.ChannelStatusManuallyDisabled,
			},
		},
	}
	assert.Equal(t, []int{1}, channel.GetAutoDisabledKeyIndexes())

	allEnabled := &Channel{
		Key:         "k0\nk1",
		ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeySize: 2},
	}
	assert.Empty(t, allEnabled.GetAutoDisabledKeyIndexes())
}
