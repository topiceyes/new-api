package controller

import (
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type dingTalkAuthUrlRequest struct {
	RedirectUri string `json:"redirect_uri"`
	State       string `json:"state"`
}

// DingTalkAuthUrl builds the enterprise authorization URL for external browsers.
// DingTalk enterprise internal apps require an application access token to
// construct the login URL, so this is handled server-side.
func DingTalkAuthUrl(c *gin.Context) {
	settings := system_setting.GetDingTalkSettings()
	if !settings.Enabled || settings.AppKey == "" || settings.AppSecret == "" || settings.CorpId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthNotEnabled, providerParams("DingTalk")),
		})
		return
	}

	var req dingTalkAuthUrlRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	provider, ok := oauth.GetProvider("dingtalk").(*oauth.DingTalkProvider)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthUnknownProvider),
		})
		return
	}

	appAccessToken, err := provider.GetAppAccessToken(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthConnectFailed, providerParams("DingTalk")),
		})
		return
	}

	authUrl := "https://login.dingtalk.com/oauth2/auth?" +
		"access_token=" + url.QueryEscape(appAccessToken) +
		"&client_id=" + url.QueryEscape(settings.AppKey) +
		"&redirect_uri=" + url.QueryEscape(req.RedirectUri) +
		"&response_type=code" +
		"&scope=" + url.QueryEscape("openid corpid") +
		"&state=" + url.QueryEscape(req.State) +
		"&prompt=consent"

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"url": authUrl,
		},
	})
}
