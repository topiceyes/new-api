package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/require"
)

// TestApplyUTLSFingerprintDialTLS uTLS 拨号真实握手:指纹开启的 transport
// 必须挂上 DialTLSContext、禁用自动 h2,并能对 httptest TLS 服务端完成完整请求。
func TestApplyUTLSFingerprintDialTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	transport := newRelayHTTPTransport()
	transport.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	policy := HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: 1, TLSFingerprint: dto.TLSFingerprintChrome}
	applyUTLSFingerprint(transport, policy, nil)

	require.NotNil(t, transport.DialTLSContext, "指纹开启时必须挂自定义 DialTLS")
	require.False(t, transport.ForceAttemptHTTP2, "自定义 DialTLS 后必须禁用自动 h2")

	client := &http.Client{Transport: transport}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err, "uTLS 握手 + 请求应成功")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 空指纹不挂 DialTLS,不影响默认握手
	plainTransport := newRelayHTTPTransport()
	plainTransport.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	applyUTLSFingerprint(plainTransport, HTTPTransportPolicy{}, nil)
	require.Nil(t, plainTransport.DialTLSContext)

	// http 代理 + 指纹:回退默认握手(不挂 DialTLS)
	proxyTransport := newRelayHTTPTransport()
	applyUTLSFingerprint(proxyTransport, policy, mustParseURL(t, "http://127.0.0.1:1"))
	require.Nil(t, proxyTransport.DialTLSContext, "http 代理应回退默认握手")
}
