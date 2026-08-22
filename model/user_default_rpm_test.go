package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDefaultRPMTestState(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	oldRedisEnabled := common.RedisEnabled
	oldDefaultUserRPM := common.DefaultUserRPM
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.DefaultUserRPM = oldDefaultUserRPM
	})
}

func TestInsertWithTxAppliesDefaultUserRPM(t *testing.T) {
	setupDefaultRPMTestState(t)
	common.DefaultUserRPM = 30

	user := User{Username: "rpm-default-user", Password: "hash"}
	require.NoError(t, user.InsertWithTx(DB, 0))

	var created User
	require.NoError(t, DB.Where("username = ?", "rpm-default-user").First(&created).Error)
	assert.Equal(t, 30, created.GetSetting().RateLimitRPM)
}

func TestInsertWithTxDefaultUserRPMZeroLeavesSettingEmpty(t *testing.T) {
	setupDefaultRPMTestState(t)
	common.DefaultUserRPM = 0

	user := User{Username: "rpm-zero-user", Password: "hash"}
	require.NoError(t, user.InsertWithTx(DB, 0))

	var created User
	require.NoError(t, DB.Where("username = ?", "rpm-zero-user").First(&created).Error)
	assert.Equal(t, 0, created.GetSetting().RateLimitRPM)
}

func TestInsertWithTxExplicitSettingBeatsDefaultUserRPM(t *testing.T) {
	setupDefaultRPMTestState(t)
	common.DefaultUserRPM = 30

	user := User{Username: "rpm-explicit-user", Password: "hash", Setting: `{"rate_limit_rpm":5}`}
	require.NoError(t, user.InsertWithTx(DB, 0))

	var created User
	require.NoError(t, DB.Where("username = ?", "rpm-explicit-user").First(&created).Error)
	assert.Equal(t, 5, created.GetSetting().RateLimitRPM)
}
