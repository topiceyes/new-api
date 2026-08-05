package oauth

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
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
	Register("feishu", &FeishuProvider{})
}

// FeishuProvider implements OAuth for Feishu (Lark) enterprise self-built app
type FeishuProvider struct{}

// Feishu API error codes relevant to membership checks.
const (
	// feishuErrUserIDInvalid is returned by contact/v3/users when the user ID
	// is unknown to the tenant — this only happens when the user was hard
	// deleted by an admin (normal resignation keeps the record queryable).
	feishuErrUserIDInvalid = 41012
	// feishuErrNoUserAuthority means the user is outside the app's contact
	// permission scope — a configuration issue, NOT proof of departure.
	feishuErrNoUserAuthority = 41050
)

type feishuAPIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// feishuUserStatus is the status object embedded in contact/v3/users responses.
type feishuUserStatus struct {
	IsFrozen    bool `json:"is_frozen"`
	IsResigned  bool `json:"is_resigned"`
	IsActivated bool `json:"is_activated"`
	IsExited    bool `json:"is_exited"`
	IsUnjoin    bool `json:"is_unjoin"`
}

func (p *FeishuProvider) GetName() string {
	return "Feishu"
}

func (p *FeishuProvider) IsEnabled() bool {
	return system_setting.GetFeishuSettings().Enabled
}

// Tenant access tokens are long-lived (~2h) and shared by the login-time
// membership recheck and the leave patrol, so cache them instead of
// re-fetching per request. Redis is preferred when available; an in-memory
// cache is the fallback for single-node deployments without Redis.
const feishuTenantTokenCacheKey = "feishu:tenant_access_token"

var (
	feishuTenantTokenMu     sync.Mutex
	feishuTenantTokenCached string
	feishuTenantTokenExpiry time.Time
)

// GetTenantAccessToken fetches the app-level tenant_access_token using
// AppId + AppSecret. Required for contact/v3 membership queries.
func (p *FeishuProvider) GetTenantAccessToken(ctx context.Context) (string, error) {
	if token := getCachedFeishuTenantToken(); token != "" {
		return token, nil
	}

	settings := system_setting.GetFeishuSettings()
	reqBody := map[string]string{
		"app_id":     settings.AppId,
		"app_secret": settings.AppSecret,
	}
	bodyBytes, err := common.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var tokenResp struct {
		feishuAPIError
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err = common.DecodeJson(res.Body, &tokenResp); err != nil {
		return "", err
	}
	if tokenResp.Code != 0 || tokenResp.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu tenant_access_token code=%d msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	// Keep a safety margin before the real expiry; enforce a sane minimum.
	ttl := time.Duration(tokenResp.Expire-300) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	cacheFeishuTenantToken(tokenResp.TenantAccessToken, ttl)
	return tokenResp.TenantAccessToken, nil
}

func getCachedFeishuTenantToken() string {
	if common.RedisEnabled {
		if token, err := common.RedisGet(feishuTenantTokenCacheKey); err == nil && token != "" {
			return token
		}
	}
	feishuTenantTokenMu.Lock()
	defer feishuTenantTokenMu.Unlock()
	if feishuTenantTokenCached != "" && time.Now().Before(feishuTenantTokenExpiry) {
		return feishuTenantTokenCached
	}
	return ""
}

func cacheFeishuTenantToken(token string, ttl time.Duration) {
	if common.RedisEnabled {
		if err := common.RedisSet(feishuTenantTokenCacheKey, token, ttl); err == nil {
			return
		}
	}
	feishuTenantTokenMu.Lock()
	feishuTenantTokenCached = token
	feishuTenantTokenExpiry = time.Now().Add(ttl)
	feishuTenantTokenMu.Unlock()
}

// CheckUserActive reports whether the Feishu user identified by unionId is
// still an active member of the organization. It returns (true, nil) when
// membership is confirmed, (false, nil) only on definitive departure signals —
// status.is_resigned / is_exited / is_frozen in a successful response, or
// error code 41012 (user hard-deleted). Every other outcome (41050 out of
// contact scope, rate limits, network failures, unexpected codes) returns
// (false, err). Callers must treat an error as "unknown" and must never
// disable or reject based on it, otherwise a permission-scope misconfiguration
// or Feishu outage would lock out active employees.
func (p *FeishuProvider) CheckUserActive(ctx context.Context, unionId string) (bool, error) {
	if unionId == "" {
		return false, fmt.Errorf("empty unionId")
	}
	tenantToken, err := p.GetTenantAccessToken(ctx)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://open.feishu.cn/open-apis/contact/v3/users/"+unionId+"?user_id_type=union_id", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+tenantToken)
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	var resp struct {
		feishuAPIError
		Data struct {
			User struct {
				Status feishuUserStatus `json:"status"`
			} `json:"user"`
		} `json:"data"`
	}
	if err = common.DecodeJson(res.Body, &resp); err != nil {
		return false, err
	}
	switch resp.Code {
	case 0:
		if resp.Data.User.Status.IsResigned || resp.Data.User.Status.IsExited || resp.Data.User.Status.IsFrozen {
			return false, nil
		}
		return true, nil
	case feishuErrUserIDInvalid:
		return false, nil
	default:
		return false, fmt.Errorf("feishu contact/v3/users code=%d msg=%s", resp.Code, resp.Msg)
	}
}

func (p *FeishuProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	settings := system_setting.GetFeishuSettings()
	reqBody := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     settings.AppId,
		"client_secret": settings.AppSecret,
		"code":          code,
	}
	bodyBytes, err := common.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/authen/v2/oauth/token",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Feishu] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Feishu"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-Feishu] ExchangeToken response status: %d", res.StatusCode)

	var tokenResp struct {
		feishuAPIError
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err = common.DecodeJson(res.Body, &tokenResp); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Feishu] ExchangeToken decode error: %s", err.Error()))
		return nil, err
	}

	if tokenResp.Code != 0 || tokenResp.AccessToken == "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Feishu] ExchangeToken failed: code=%d msg=%s", tokenResp.Code, tokenResp.Msg))
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Feishu"})
	}

	logger.LogDebug(ctx, "[OAuth-Feishu] ExchangeToken success")

	return &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    "Bearer",
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
	}, nil
}

func (p *FeishuProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	logger.LogDebug(ctx, "[OAuth-Feishu] GetUserInfo: fetching user info")

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://open.feishu.cn/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Feishu] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Feishu"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-Feishu] GetUserInfo response status: %d", res.StatusCode)

	var userInfoResp struct {
		feishuAPIError
		Data struct {
			UnionId string `json:"union_id"`
			OpenId  string `json:"open_id"`
			Name    string `json:"name"`
			EnName  string `json:"en_name"`
			Email   string `json:"email"`
		} `json:"data"`
	}
	if err = common.DecodeJson(res.Body, &userInfoResp); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Feishu] GetUserInfo decode error: %s", err.Error()))
		return nil, err
	}

	if userInfoResp.Code != 0 {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Feishu] GetUserInfo failed: code=%d msg=%s", userInfoResp.Code, userInfoResp.Msg))
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, nil)
	}

	user := userInfoResp.Data
	if user.UnionId == "" {
		logger.LogError(ctx, "[OAuth-Feishu] GetUserInfo failed: empty union_id")
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "Feishu"})
	}

	// Verify the employee is still in the organization before letting the
	// login proceed. Only a definitive departure signal blocks login; when
	// membership cannot be determined (network or permission-scope issues) the
	// login is allowed — a Feishu API hiccup must not lock out active staff.
	if active, err := p.CheckUserActive(ctx, user.UnionId); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-Feishu] CheckUserActive inconclusive for union_id=%s: %s; allowing login", user.UnionId, err.Error()))
	} else if !active {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-Feishu] union_id=%s is no longer in the organization; login denied", user.UnionId))
		return nil, NewOAuthError(i18n.MsgFeishuUserLeftOrg, nil)
	}

	displayName := user.Name
	if displayName == "" {
		displayName = user.EnName
	}

	logger.LogDebug(ctx, "[OAuth-Feishu] GetUserInfo success: union_id=%s, name=%s", user.UnionId, displayName)

	return &OAuthUser{
		ProviderUserID: user.UnionId,
		DisplayName:    displayName,
		Email:          user.Email,
	}, nil
}

func (p *FeishuProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsFeishuIdAlreadyTaken(providerUserID)
}

func (p *FeishuProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.FeishuId = providerUserID
	return user.FillUserByFeishuId()
}

func (p *FeishuProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.FeishuId = providerUserID
}

func (p *FeishuProvider) GetProviderPrefix() string {
	return "feishu_"
}
