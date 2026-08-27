package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHTTPTransportPolicyTLSFingerprint(t *testing.T) {
	// 合法值归一小写并保留
	p := NormalizeHTTPTransportPolicy(dto.ChannelSettings{TLSFingerprint: " Chrome "})
	require.Equal(t, dto.TLSFingerprintChrome, p.TLSFingerprint)
	require.Equal(t, 1, p.Shards, "指纹生效时强制单分片(http/1.1)")

	// h2 分片与指纹共存时,指纹胜出(降为 http/1.1 单连接)
	p = NormalizeHTTPTransportPolicy(dto.ChannelSettings{TLSFingerprint: "ios", HTTP2ConnectionShards: 8})
	require.Equal(t, dto.TLSFingerprintIOS, p.TLSFingerprint)
	require.Equal(t, 1, p.Shards)

	// 未知值钳为关闭
	p = NormalizeHTTPTransportPolicy(dto.ChannelSettings{TLSFingerprint: "netscape"})
	require.Empty(t, p.TLSFingerprint)
	require.Equal(t, 1, p.Shards)

	// 空值不影响原有协议/分片语义
	p = NormalizeHTTPTransportPolicy(dto.ChannelSettings{HTTP2ConnectionShards: 4})
	require.Empty(t, p.TLSFingerprint)
	require.Equal(t, 4, p.Shards)
}

func TestUTLSClientHelloIDMapping(t *testing.T) {
	for _, fp := range []string{
		dto.TLSFingerprintChrome, dto.TLSFingerprintSafari, dto.TLSFingerprintIOS,
		dto.TLSFingerprintFirefox, dto.TLSFingerprintEdge, dto.TLSFingerprintAndroid,
		dto.TLSFingerprintRandomized,
	} {
		id, ok := utlsClientHelloID(fp)
		require.True(t, ok, fp)
		assert.NotEmpty(t, id.Client, fp)
	}
	_, ok := utlsClientHelloID("bogus")
	assert.False(t, ok)
}

func TestHTTPTransportPolicyCacheKeyIncludesFingerprint(t *testing.T) {
	a := HTTPTransportPolicy{Protocol: "auto", Shards: 1}
	b := HTTPTransportPolicy{Protocol: "auto", Shards: 1, TLSFingerprint: dto.TLSFingerprintChrome}
	assert.NotEqual(t, a.cacheKeyPart(), b.cacheKeyPart(), "不同指纹必须得到不同客户端缓存键")
}
