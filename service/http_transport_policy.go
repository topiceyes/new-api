package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"

	utls "github.com/refraction-networking/utls"
)

// HTTPTransportPolicy is the runtime-normalized outbound HTTP transport policy
// for a channel. Unknown or out-of-range stored values are clamped safely.
type HTTPTransportPolicy struct {
	Protocol string // dto.HTTPProtocolAuto or dto.HTTPProtocolHTTP1
	Shards   int    // 1..dto.MaxHTTP2ConnectionShards
	// TLSFingerprint 为空表示 Go 默认握手;否则用 uTLS 模拟对应客户端的
	// ClientHello(见 http_client.go utlsClientHelloID)。
	TLSFingerprint string
}

var httpTransportPolicyWarnings sync.Map

func defaultHTTPTransportPolicy() HTTPTransportPolicy {
	return HTTPTransportPolicy{
		Protocol: dto.HTTPProtocolAuto,
		Shards:   1,
	}
}

// utlsClientHelloID 把渠道配置的指纹值映射到 uTLS 预设;未知值返回 false。
func utlsClientHelloID(fingerprint string) (utls.ClientHelloID, bool) {
	switch fingerprint {
	case dto.TLSFingerprintChrome:
		return utls.HelloChrome_Auto, true
	case dto.TLSFingerprintSafari:
		return utls.HelloSafari_Auto, true
	case dto.TLSFingerprintIOS:
		return utls.HelloIOS_Auto, true
	case dto.TLSFingerprintFirefox:
		return utls.HelloFirefox_Auto, true
	case dto.TLSFingerprintEdge:
		return utls.HelloEdge_Auto, true
	case dto.TLSFingerprintAndroid:
		return utls.HelloAndroid_11_OkHttp, true
	case dto.TLSFingerprintRandomized:
		return utls.HelloRandomized, true
	default:
		return utls.ClientHelloID{}, false
	}
}

// NormalizeHTTPTransportPolicy converts channel settings into a safe runtime policy.
// Invalid stored values never panic; they clamp to defaults and warn once per bad value.
func NormalizeHTTPTransportPolicy(settings dto.ChannelSettings) HTTPTransportPolicy {
	policy := defaultHTTPTransportPolicy()

	fingerprint := strings.ToLower(strings.TrimSpace(settings.TLSFingerprint))
	if fingerprint != "" {
		if _, ok := utlsClientHelloID(fingerprint); !ok {
			warnHTTPTransportPolicyOnce("tls_fingerprint", settings.TLSFingerprint)
			fingerprint = ""
		}
	}
	policy.TLSFingerprint = fingerprint

	protocol := strings.ToLower(strings.TrimSpace(settings.HTTPProtocol))
	switch protocol {
	case "", dto.HTTPProtocolAuto:
		policy.Protocol = dto.HTTPProtocolAuto
	case dto.HTTPProtocolHTTP1:
		policy.Protocol = dto.HTTPProtocolHTTP1
	default:
		warnHTTPTransportPolicyOnce("http_protocol", settings.HTTPProtocol)
		policy.Protocol = dto.HTTPProtocolAuto
	}

	shards := settings.HTTP2ConnectionShards
	switch {
	case shards == 0:
		policy.Shards = 1
	case shards < 1:
		warnHTTPTransportPolicyOnce("http2_connection_shards", fmt.Sprintf("%d", shards))
		policy.Shards = 1
	case shards > dto.MaxHTTP2ConnectionShards:
		warnHTTPTransportPolicyOnce("http2_connection_shards", fmt.Sprintf("%d", shards))
		policy.Shards = dto.MaxHTTP2ConnectionShards
	default:
		policy.Shards = shards
	}

	if policy.Protocol == dto.HTTPProtocolHTTP1 {
		if settings.HTTP2ConnectionShards > 1 {
			warnHTTPTransportPolicyOnce(
				"http_protocol+http2_connection_shards",
				fmt.Sprintf("%s+%d", dto.HTTPProtocolHTTP1, settings.HTTP2ConnectionShards),
			)
		}
		policy.Shards = 1
	}
	if policy.Shards < 1 {
		policy.Shards = 1
	}
	if policy.TLSFingerprint != "" {
		// uTLS 走 DialTLS 仅 HTTP/1.1,h2 分片无意义(放在最后,防被上方
		// shards 解析覆盖)。
		policy.Shards = 1
	}
	return policy
}

func warnHTTPTransportPolicyOnce(field, value string) {
	key := field + "=" + value
	if _, loaded := httpTransportPolicyWarnings.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	logger.LogWarn(
		context.Background(),
		fmt.Sprintf("invalid channel http transport setting clamped: %s=%q", field, value),
	)
}

func (p HTTPTransportPolicy) cacheKeyPart() string {
	return fmt.Sprintf("%s|%d|%s", p.Protocol, p.Shards, p.TLSFingerprint)
}

func (p HTTPTransportPolicy) String() string {
	return p.cacheKeyPart()
}
