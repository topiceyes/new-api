package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useStatusVersionTestEnv(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "status-version-gating-test-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})
}

func getStatusVersionPresence(t *testing.T, authHeader string) (hasVersion bool, version any) {
	t.Helper()
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousMap })

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	if authHeader != "" {
		context.Request.Header.Set("Authorization", authHeader)
	}

	GetStatus(context)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	version, hasVersion = payload.Data["version"]
	return hasVersion, version
}

func TestGetStatusOmitsVersionForAnonymousCallers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hasVersion, _ := getStatusVersionPresence(t, "")
	assert.False(t, hasVersion)
}

func TestGetStatusOmitsVersionForGarbageToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useStatusVersionTestEnv(t)
	hasVersion, _ := getStatusVersionPresence(t, "Bearer not-a-real-token")
	assert.False(t, hasVersion)
}

func TestGetStatusRevealsVersionToDashboardSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useStatusVersionTestEnv(t)
	previousVersion := common.Version
	common.Version = "v9.9.9-test"
	t.Cleanup(func() { common.Version = previousVersion })

	user := &model.User{
		Username: "status-version-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(user).Error)
	session, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "agent")
	require.NoError(t, err)

	hasVersion, version := getStatusVersionPresence(t, "Bearer "+session.AccessToken)
	assert.True(t, hasVersion)
	assert.Equal(t, "v9.9.9-test", version)
}
