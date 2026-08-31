package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMultiKeyChannel() *model.Channel {
	return &model.Channel{
		Id:  1,
		Key: "key-0\nkey-1\nkey-2",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModeRandom,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
}

// 健康检查 pin 住被禁用的 key 下标时,必须原样返回该 key(跳过启用状态过滤)。
func TestSelectChannelKeyPinsDisabledIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(constant.ContextKeyChannelTestPinKeyIndex), 1)

	key, index, err := selectChannelKey(c, newMultiKeyChannel())

	require.Nil(t, err)
	assert.Equal(t, "key-1", key)
	assert.Equal(t, 1, index)
}

// pin 下标越界(key 列表被编辑过)时回落原选 key 逻辑,只能拿到启用中的 key。
func TestSelectChannelKeyPinOutOfRangeFallsBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(constant.ContextKeyChannelTestPinKeyIndex), 9)

	key, _, err := selectChannelKey(c, newMultiKeyChannel())

	require.Nil(t, err)
	assert.NotEqual(t, "key-1", key)
	assert.Contains(t, []string{"key-0", "key-2"}, key)
}

// 不 pin 时走原逻辑:轮询只在启用的 key 中选择。
func TestSelectChannelKeyWithoutPinUsesEnabledKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	key, _, err := selectChannelKey(c, newMultiKeyChannel())

	require.Nil(t, err)
	assert.Contains(t, []string{"key-0", "key-2"}, key)
}
