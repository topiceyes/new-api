package oauth

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func init() {
	Register("dingtalk", &DingTalkProvider{})
}

// DingTalkProvider implements OAuth for DingTalk enterprise internal app
type DingTalkProvider struct{}

type dingTalkUserInfo struct {
	UnionId string `json:"unionId"`
	OpenId  string `json:"openId"`
	Nick    string `json:"nick"`
	Email   string `json:"email"`
}

func (p *DingTalkProvider) GetName() string {
	return "DingTalk"
}

func (p *DingTalkProvider) IsEnabled() bool {
	return system_setting.GetDingTalkSettings().Enabled
}

// App access tokens are long-lived (~2h) and shared by the login flow, the
// membership check and the daily leave audit, so cache them instead of
// re-fetching per request. Redis is preferred when available; an in-memory
// cache is the fallback for single-node deployments without Redis.
const dingTalkAppTokenCacheKey = "dingtalk:app_access_token"

var (
	dingTalkAppTokenMu     sync.Mutex
	dingTalkAppTokenCached string
	dingTalkAppTokenExpiry time.Time
)

// GetAppAccessToken fetches application-level access token using AppKey + AppSecret.
// Required for external-browser enterprise authorization URLs.
func (p *DingTalkProvider) GetAppAccessToken(ctx context.Context) (string, error) {
	if token := getCachedDingTalkAppToken(); token != "" {
		return token, nil
	}

	settings := system_setting.GetDingTalkSettings()
	reqBody := map[string]string{
		"appKey":    settings.AppKey,
		"appSecret": settings.AppSecret,
	}
	bodyBytes, err := common.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dingtalk.com/v1.0/oauth2/accessToken", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err = common.DecodeJson(res.Body, &tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty app access token")
	}

	// Keep a safety margin before the real expiry; enforce a sane minimum.
	ttl := time.Duration(tokenResp.ExpireIn-300) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	cacheDingTalkAppToken(tokenResp.AccessToken, ttl)
	return tokenResp.AccessToken, nil
}

func getCachedDingTalkAppToken() string {
	if common.RedisEnabled {
		if token, err := common.RedisGet(dingTalkAppTokenCacheKey); err == nil && token != "" {
			return token
		}
	}
	dingTalkAppTokenMu.Lock()
	defer dingTalkAppTokenMu.Unlock()
	if dingTalkAppTokenCached != "" && time.Now().Before(dingTalkAppTokenExpiry) {
		return dingTalkAppTokenCached
	}
	return ""
}

func cacheDingTalkAppToken(token string, ttl time.Duration) {
	if common.RedisEnabled {
		if err := common.RedisSet(dingTalkAppTokenCacheKey, token, ttl); err == nil {
			return
		}
	}
	dingTalkAppTokenMu.Lock()
	dingTalkAppTokenCached = token
	dingTalkAppTokenExpiry = time.Now().Add(ttl)
	dingTalkAppTokenMu.Unlock()
}

// dingTalkErrUserNotFound is the errcode topapi/user/getbyunionid returns when
// the unionId matches no member of the organization (the employee left).
const dingTalkErrUserNotFound = 60121

// CheckUserActive reports whether the DingTalk user identified by unionId is
// still a member of the organization. It returns (true, nil) when membership
// is confirmed, (false, nil) when DingTalk explicitly reports the user as not
// found (errcode 60121, i.e. the employee left), and (false, err) for any
// other outcome — network failure, missing address-book permission, or an
// unexpected errcode. Callers must treat an error as "unknown" and must never
// disable or reject based on it, otherwise a DingTalk outage or permission
// gap would lock out active employees.
func (p *DingTalkProvider) CheckUserActive(ctx context.Context, unionId string) (bool, error) {
	if unionId == "" {
		return false, fmt.Errorf("empty unionId")
	}
	appToken, err := p.GetAppAccessToken(ctx)
	if err != nil {
		return false, err
	}

	bodyBytes, err := common.Marshal(map[string]string{"unionid": unionId})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token="+url.QueryEscape(appToken),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			UserID string `json:"userid"`
		} `json:"result"`
	}
	if err = common.DecodeJson(res.Body, &resp); err != nil {
		return false, err
	}
	switch resp.ErrCode {
	case 0:
		return true, nil
	case dingTalkErrUserNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("dingtalk getbyunionid errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
	}
}

func (p *DingTalkProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	settings := system_setting.GetDingTalkSettings()
	reqBody := map[string]string{
		"clientId":     settings.AppKey,
		"clientSecret": settings.AppSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}
	bodyBytes, err := common.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dingtalk.com/v1.0/oauth2/userAccessToken", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-DingTalk] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "DingTalk"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-DingTalk] ExchangeToken response status: %d", res.StatusCode)

	var tokenResp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err = common.DecodeJson(res.Body, &tokenResp); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-DingTalk] ExchangeToken decode error: %s", err.Error()))
		return nil, err
	}

	if tokenResp.AccessToken == "" {
		logger.LogError(ctx, "[OAuth-DingTalk] ExchangeToken failed: empty access token")
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "DingTalk"})
	}

	logger.LogDebug(ctx, "[OAuth-DingTalk] ExchangeToken success")

	return &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    "Bearer",
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
	}, nil
}

func (p *DingTalkProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	logger.LogDebug(ctx, "[OAuth-DingTalk] GetUserInfo: fetching user info")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.dingtalk.com/v1.0/contact/users/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-acs-dingtalk-access-token", token.AccessToken)
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-DingTalk] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "DingTalk"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-DingTalk] GetUserInfo response status: %d", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-DingTalk] GetUserInfo failed: status=%d", res.StatusCode))
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, nil)
	}

	var user dingTalkUserInfo
	if err = common.DecodeJson(res.Body, &user); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-DingTalk] GetUserInfo decode error: %s", err.Error()))
		return nil, err
	}

	if user.UnionId == "" {
		logger.LogError(ctx, "[OAuth-DingTalk] GetUserInfo failed: empty unionId")
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "DingTalk"})
	}

	// Verify the employee is still in the organization before letting the
	// login proceed. Only a definitive "not found" answer blocks login; when
	// membership cannot be determined (network or permission issues) the
	// login is allowed — a DingTalk API hiccup must not lock out active staff.
	if active, err := p.CheckUserActive(ctx, user.UnionId); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-DingTalk] CheckUserActive inconclusive for unionId=%s: %s; allowing login", user.UnionId, err.Error()))
	} else if !active {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-DingTalk] unionId=%s is no longer in the organization; login denied", user.UnionId))
		return nil, NewOAuthError(i18n.MsgOAuthUserLeftOrg, nil)
	}

	logger.LogDebug(ctx, "[OAuth-DingTalk] GetUserInfo success: unionId=%s, nick=%s", user.UnionId, user.Nick)

	return &OAuthUser{
		ProviderUserID: user.UnionId,
		DisplayName:    user.Nick,
		Email:          user.Email,
	}, nil
}

func (p *DingTalkProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsDingTalkIdAlreadyTaken(providerUserID)
}

func (p *DingTalkProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.DingTalkId = providerUserID
	return user.FillUserByDingTalkId()
}

func (p *DingTalkProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.DingTalkId = providerUserID
}

func (p *DingTalkProvider) GetProviderPrefix() string {
	return "dingtalk_"
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *DingTalkProvider) ProviderUserIDColumn() string {
	return "dingtalk_id"
}
