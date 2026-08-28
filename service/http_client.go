package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

var (
	httpClient              *http.Client
	ssrfProtectedHTTPClient *http.Client
	proxyClients            = proxyHTTPClientCache{
		clients: make(map[string]*http.Client),
		aliases: make(map[string]string),
	}
	legacyProxyURLWarnings sync.Map
)

type proxyHTTPClientCache struct {
	mutex   sync.RWMutex
	clients map[string]*http.Client
	aliases map[string]string // rawProxyURL -> canonicalProxyURL
}

type proxyURLConfig struct {
	parsedURL *url.URL
	cacheKey  string
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	urlStr := req.URL.String()
	if err := validateURLWithCurrentFetchSetting(urlStr, true); err != nil {
		return fmt.Errorf("redirect to %s blocked: %v", urlStr, err)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func checkProtectedFetchRedirect(req *http.Request, via []*http.Request) error {
	urlStr := req.URL.String()
	if err := ValidateSSRFProtectedFetchURL(urlStr); err != nil {
		return fmt.Errorf("redirect to %s blocked: %v", urlStr, err)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func validateURLWithCurrentFetchSetting(urlStr string, applyDomainIPFilter bool) error {
	fetchSetting := system_setting.GetFetchSetting()
	return common.ValidateURLWithFetchSetting(urlStr, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, applyDomainIPFilter && fetchSetting.ApplyIPFilterForDomain)
}

func ValidateSSRFProtectedFetchURL(urlStr string) error {
	return validateURLWithCurrentFetchSetting(urlStr, true)
}

func newRelayHTTPTransport() *http.Transport {
	var transport *http.Transport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok && defaultTransport != nil {
		transport = defaultTransport.Clone()
	} else {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		}
	}
	transport.MaxIdleConns = common.RelayMaxIdleConns
	transport.MaxIdleConnsPerHost = common.RelayMaxIdleConnsPerHost
	transport.IdleConnTimeout = time.Duration(common.RelayIdleConnTimeout) * time.Second
	transport.ForceAttemptHTTP2 = true
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	return transport
}

func newRelayHTTPClient(transport http.RoundTripper) *http.Client {
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	if common.RelayTimeout != 0 {
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	return client
}

func clientCacheKey(proxyCacheKey string, policy HTTPTransportPolicy) string {
	return proxyCacheKey + "\x00" + policy.cacheKeyPart()
}

func InitHttpClient() {
	policy := defaultHTTPTransportPolicy()
	httpClient = newDirectHTTPClient(policy, nil)
	proxyClients.store(clientCacheKey("", policy), httpClient)
	ssrfProtectedHTTPClient = newProtectedFetchHTTPClient()
}

// GetHttpClient returns the general outbound client used by relay/provider
// integrations. Do not attach the SSRF-protected dialer here: provider base URLs
// are root/operator-managed deployment targets, not arbitrary user-controlled
// input, and may legitimately point at private networks, private-link endpoints,
// self-hosted services, or local proxies. Code paths that fetch arbitrary
// user-controlled URLs must use GetSSRFProtectedHTTPClient or
// ValidateSSRFProtectedFetchURL instead.
func GetHttpClient() *http.Client {
	return httpClient
}

// GetSSRFProtectedHTTPClient 返回带拨号时 SSRF 校验的客户端。
// ssrfProtectedHTTPClient 由 InitHttpClient 在启动时初始化，运行期只读。
func GetSSRFProtectedHTTPClient() *http.Client {
	if fetchSetting := system_setting.GetFetchSetting(); fetchSetting != nil && !fetchSetting.EnableSSRFProtection {
		return GetHttpClient()
	}
	return ssrfProtectedHTTPClient
}

func newProxyURLConfig(parsedURL *url.URL) *proxyURLConfig {
	return &proxyURLConfig{
		parsedURL: parsedURL,
		cacheKey:  parsedURL.String(),
	}
}

func warnLegacyProxyURLOnce(config *proxyURLConfig) {
	if _, loaded := legacyProxyURLWarnings.LoadOrStore(config.cacheKey, struct{}{}); loaded {
		return
	}
	logger.LogWarn(
		context.Background(),
		fmt.Sprintf(
			"legacy proxy URL suffix ignored at runtime: scheme=%s host=%s; update the channel proxy setting",
			config.parsedURL.Scheme,
			config.parsedURL.Host,
		),
	)
}

// NormalizeProxyURL validates a proxy URL using runtime-compatible rules and returns its canonical cache key.
func NormalizeProxyURL(rawProxyURL string) (string, error) {
	parsedURL, legacySuffixStripped, err := common.ParseProxyURLRuntime(rawProxyURL)
	if err != nil {
		return "", err
	}
	if parsedURL == nil {
		return "", nil
	}
	config := newProxyURLConfig(parsedURL)
	if legacySuffixStripped {
		warnLegacyProxyURLOnce(config)
	}
	return config.cacheKey, nil
}

// ValidateProxyURL validates a channel proxy URL without connecting to it.
func ValidateProxyURL(rawProxyURL string) error {
	_, err := common.ParseProxyURLStrict(rawProxyURL)
	return err
}

func (cache *proxyHTTPClientCache) store(fullKey string, client *http.Client) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.clients[fullKey] = client
}

func (cache *proxyHTTPClientCache) resolveProxyKey(rawProxyURL string) string {
	if canonicalKey, ok := cache.aliases[rawProxyURL]; ok {
		return canonicalKey
	}
	return rawProxyURL
}

func (cache *proxyHTTPClientCache) get(rawProxyURL string, policy HTTPTransportPolicy) (*http.Client, bool) {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	proxyKey := cache.resolveProxyKey(rawProxyURL)
	client, ok := cache.clients[clientCacheKey(proxyKey, policy)]
	return client, ok
}

func (cache *proxyHTTPClientCache) getOrCreate(
	rawProxyURL string,
	config *proxyURLConfig,
	policy HTTPTransportPolicy,
	factory func() (*http.Client, error),
) (*http.Client, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	proxyKey := ""
	if config != nil {
		proxyKey = config.cacheKey
		cache.aliases[rawProxyURL] = proxyKey
	} else if rawProxyURL != "" {
		proxyKey = cache.resolveProxyKey(rawProxyURL)
	}
	fullKey := clientCacheKey(proxyKey, policy)
	if client, ok := cache.clients[fullKey]; ok {
		return client, nil
	}

	client, err := factory()
	if err != nil {
		return nil, err
	}
	cache.clients[fullKey] = client
	return client, nil
}

func (cache *proxyHTTPClientCache) removeProxy(proxyCacheKey string) []*http.Client {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	removed := make([]*http.Client, 0)
	prefix := proxyCacheKey + "\x00"
	for key, client := range cache.clients {
		if strings.HasPrefix(key, prefix) {
			removed = append(removed, client)
			delete(cache.clients, key)
		}
	}
	for alias, canonicalKey := range cache.aliases {
		if canonicalKey == proxyCacheKey {
			delete(cache.aliases, alias)
		}
	}
	return removed
}

func (cache *proxyHTTPClientCache) reset() map[string]*http.Client {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	oldClients := cache.clients
	cache.clients = make(map[string]*http.Client)
	cache.aliases = make(map[string]string)
	return oldClients
}

func configureProxyTransport(transport *http.Transport, proxyURL *url.URL) error {
	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
		return nil
	case "socks5", "socks5h":
		transport.Proxy = nil
		forwardDialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialer, err := proxy.FromURL(proxyURL, forwardDialer)
		if err != nil {
			return err
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return fmt.Errorf("SOCKS proxy dialer does not support context cancellation")
		}
		transport.DialContext = contextDialer.DialContext
		return nil
	default:
		return fmt.Errorf("unsupported proxy scheme")
	}
}

func newTransportFactory(proxyURL *url.URL, tlsConfig *tls.Config) (func() *http.Transport, error) {
	// Validate proxy configuration once before creating shard transports.
	if proxyURL != nil {
		probe := newRelayHTTPTransport()
		if err := configureProxyTransport(probe, proxyURL); err != nil {
			return nil, err
		}
	}
	return func() *http.Transport {
		transport := newRelayHTTPTransport()
		if proxyURL != nil {
			_ = configureProxyTransport(transport, proxyURL)
		} else {
			transport.Proxy = http.ProxyFromEnvironment
		}
		if tlsConfig != nil {
			transport.TLSClientConfig = tlsConfig.Clone()
		}
		return transport
	}, nil
}

// applyUTLSFingerprint 把 uTLS 拨号挂到 transport 的 DialTLSContext 上,出站
// HTTPS 握手即模拟所选客户端的 ClientHello(JA3/JA4)。仅 HTTP/1.1(ALPN 只协商
// http/1.1,h2 over uTLS 需另行接管 http2.Transport,不在本期);http/https 代理
// 走 CONNECT,其 TLS 在隧道另一端,保持 Go 默认握手并告警一次;socks5 代理的
// DialContext 不受影响,uTLS 照常生效。
func applyUTLSFingerprint(transport *http.Transport, policy HTTPTransportPolicy, proxyURL *url.URL) {
	if policy.TLSFingerprint == "" {
		return
	}
	if proxyURL != nil && (proxyURL.Scheme == "http" || proxyURL.Scheme == "https") {
		warnHTTPTransportPolicyOnce("tls_fingerprint+http_proxy", proxyURL.Scheme)
		return
	}
	helloID, ok := utlsClientHelloID(policy.TLSFingerprint)
	if !ok {
		return
	}
	// 预设指纹(Chrome/Safari 等)的 ClientHello 自带 ALPN 扩展(h2+http/1.1),
	// 优先级高于 utls.Config.NextProtos——上游会协商出 h2,而自定义
	// DialTLSContext 后 transport 只会说 HTTP/1.1,于是把上游的 HTTP/2
	// SETTINGS 帧当成畸形响应直接断连。对固定预设做 spec 手术,把 ALPN
	// 替换为仅 http/1.1;HelloRandomized 无固定 spec,ALPN 由
	// config.NextProtos 生成,不受影响。
	// 注意:spec 必须在每次握手时重新生成——ApplyPreset 会让 uconn 持有
	// spec 内的扩展指针并就地改写,共享同一个 spec 第二次握手即
	// tls: internal error(实测复现)。
	presetSupported := false
	if _, err := utls.UTLSIdToSpec(helloID); err == nil {
		presetSupported = true
	}
	// HelloRandomized 走 ApplyPreset(HelloRandomized) 等价路径:直接让 uTLS
	// 内置随机化,不经过 HelloCustom。
	if helloID == utls.HelloRandomized || helloID == utls.HelloRandomizedALPN || helloID == utls.HelloRandomizedNoALPN {
		presetSupported = false
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	baseTLS := transport.TLSClientConfig.Clone()
	insecure := baseTLS.InsecureSkipVerify
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		// 复用 transport 的 DialContext(socks5 代理即走代理链路)。
		rawConn, err := transport.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		// 继承原 TLSClientConfig 的信任链(RootCAs/客户端证书)与校验开关,
		// 否则自定义 CA(企业代理/私有上游)与 insecure 配置会失效。
		// 客户端证书经 GetClientCertificate 桥接(utls.Certificate 与
		// crypto/tls.Certificate 类型不同,不能直接赋值)。
		utlsConfig := &utls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecure,
			RootCAs:            baseTLS.RootCAs,
			// 只协商 http/1.1:HTTP/2 over uTLS 需要接管 http2 流多路复用,
			// 与现有 sharded transport 体系冲突,超本期范围。
			NextProtos: []string{"http/1.1"},
		}
		if len(baseTLS.Certificates) > 0 || baseTLS.GetClientCertificate != nil {
			certs := baseTLS.Certificates
			fallback := baseTLS.GetClientCertificate
			utlsConfig.GetClientCertificate = func(*utls.CertificateRequestInfo) (*utls.Certificate, error) {
				var picked *tls.Certificate
				if fallback != nil {
					cert, err := fallback(nil)
					if err != nil || cert == nil {
						return nil, err
					}
					picked = cert
				} else if len(certs) > 0 {
					picked = &certs[0]
				}
				if picked == nil {
					return nil, nil
				}
				converted := utls.Certificate{
					Certificate: picked.Certificate,
					PrivateKey:  picked.PrivateKey,
					Leaf:        picked.Leaf,
				}
				return &converted, nil
			}
		}
		var tlsConn *utls.UConn
		if presetSupported {
			spec, err := utls.UTLSIdToSpec(helloID)
			if err != nil {
				_ = rawConn.Close()
				return nil, err
			}
			for i, ext := range spec.Extensions {
				if _, isALPN := ext.(*utls.ALPNExtension); isALPN {
					spec.Extensions[i] = &utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}}
				}
			}
			tlsConn = utls.UClient(rawConn, utlsConfig, utls.HelloCustom)
			if err := tlsConn.ApplyPreset(&spec); err != nil {
				_ = rawConn.Close()
				return nil, err
			}
		} else {
			tlsConn = utls.UClient(rawConn, utlsConfig, helloID)
		}
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}
	// 自定义 DialTLS 后 net/http 不再自动启用 h2。
	transport.ForceAttemptHTTP2 = false
}

func newHTTPClientFromPolicy(policy HTTPTransportPolicy, proxyURL *url.URL, tlsConfig *tls.Config) (*http.Client, error) {
	factory, err := newTransportFactory(proxyURL, tlsConfig)
	if err != nil {
		return nil, err
	}
	fingerprintFactory := func() *http.Transport {
		transport := factory()
		applyUTLSFingerprint(transport, policy, proxyURL)
		return transport
	}
	return newHTTPClientFromTransportFactory(policy, fingerprintFactory), nil
}

func newHTTPClientFromTransportFactory(policy HTTPTransportPolicy, factory func() *http.Transport) *http.Client {
	if policy.Shards < 1 {
		policy.Shards = 1
	}
	if policy.Protocol == dto.HTTPProtocolHTTP1 || policy.Shards == 1 {
		transport := factory()
		applyHTTPTransportPolicy(transport, policy)
		return newRelayHTTPClient(transport)
	}
	shardedFactory := func() *http.Transport {
		transport := factory()
		applyHTTPTransportPolicy(transport, policy)
		return transport
	}
	return newRelayHTTPClient(newShardedRoundTripper(policy, shardedFactory))
}

func newDirectHTTPClient(policy HTTPTransportPolicy, tlsConfig *tls.Config) *http.Client {
	client, err := newHTTPClientFromPolicy(policy, nil, tlsConfig)
	if err != nil {
		// Direct clients cannot fail proxy configuration.
		transport := newRelayHTTPTransport()
		applyHTTPTransportPolicy(transport, policy)
		return newRelayHTTPClient(transport)
	}
	return client
}

// newHTTPClientWithPolicyAndTLS is a test seam that builds a never-used transport
// stack with the given policy and TLS config (for httptest certificate trust).
func newHTTPClientWithPolicyAndTLS(policy HTTPTransportPolicy, tlsConfig *tls.Config) *http.Client {
	return newDirectHTTPClient(policy, tlsConfig)
}

func newProxyHTTPClient(proxyURL *url.URL) (*http.Client, error) {
	return newHTTPClientFromPolicy(defaultHTTPTransportPolicy(), proxyURL, nil)
}

// GetHttpClientWithProxy returns the default client or a cached proxy-enabled client.
func GetHttpClientWithProxy(rawProxyURL string) (*http.Client, error) {
	return GetHttpClientWithProxySettings(rawProxyURL, dto.ChannelSettings{})
}

// GetHttpClientWithProxySettings returns a cached HTTP client for the proxy URL and
// channel transport settings. Default auto + 1 shard shares the same client pool as
// GetHttpClientWithProxy / GetHttpClient for the empty-proxy case.
func GetHttpClientWithProxySettings(rawProxyURL string, settings dto.ChannelSettings) (*http.Client, error) {
	policy := NormalizeHTTPTransportPolicy(settings)
	trimmedProxyURL := strings.TrimSpace(rawProxyURL)

	if trimmedProxyURL == "" {
		return getOrCreateDirectClient(policy)
	}

	if client, ok := proxyClients.get(trimmedProxyURL, policy); ok {
		return client, nil
	}

	parsedURL, legacySuffixStripped, err := common.ParseProxyURLRuntime(trimmedProxyURL)
	if err != nil {
		return nil, err
	}
	config := newProxyURLConfig(parsedURL)
	if legacySuffixStripped {
		warnLegacyProxyURLOnce(config)
	}
	return proxyClients.getOrCreate(trimmedProxyURL, config, policy, func() (*http.Client, error) {
		return newHTTPClientFromPolicy(policy, config.parsedURL, nil)
	})
}

func getOrCreateDirectClient(policy HTTPTransportPolicy) (*http.Client, error) {
	defaultPolicy := defaultHTTPTransportPolicy()
	if policy == defaultPolicy {
		if client := GetHttpClient(); client != nil {
			return client, nil
		}
		// Compatibility with pre-init callers: never assign httpClient outside InitHttpClient.
		return http.DefaultClient, nil
	}

	if client, ok := proxyClients.get("", policy); ok {
		return client, nil
	}
	return proxyClients.getOrCreate("", nil, policy, func() (*http.Client, error) {
		return newDirectHTTPClient(policy, nil), nil
	})
}

// InvalidateProxyClient removes every cached policy variant for one proxy and
// closes their idle connections (including all HTTP/2 shards).
func InvalidateProxyClient(rawProxyURL string) {
	parsedURL, legacySuffixStripped, err := common.ParseProxyURLRuntime(rawProxyURL)
	if err != nil || parsedURL == nil {
		return
	}
	config := newProxyURLConfig(parsedURL)
	if legacySuffixStripped {
		warnLegacyProxyURLOnce(config)
	}
	for _, client := range proxyClients.removeProxy(config.cacheKey) {
		client.CloseIdleConnections()
	}
}

// ResetProxyClientCache clears cached proxy and non-default direct policy clients
// and closes idle connections on every transport/shard. The package-level default
// httpClient pointer stays stable after InitHttpClient; it is only closed and
// re-registered in the policy cache so concurrent GetHttpClient readers never race
// a pointer replacement.
func ResetProxyClientCache() {
	defaultClient := httpClient
	for _, client := range proxyClients.reset() {
		client.CloseIdleConnections()
	}
	if defaultClient == nil {
		return
	}
	defaultClient.CloseIdleConnections()
	proxyClients.store(clientCacheKey("", defaultHTTPTransportPolicy()), defaultClient)
}

// NewProxyHttpClient is kept for compatibility.
// Deprecated: use GetHttpClientWithProxy.
func NewProxyHttpClient(proxyURL string) (*http.Client, error) {
	return GetHttpClientWithProxy(proxyURL)
}
