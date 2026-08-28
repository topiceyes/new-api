package service

import (
	"crypto/tls"
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

// TestUTLSFingerprintNegotiatesHTTP1Only 回归:预设指纹的 ClientHello 自带
// ALPN(h2+http/1.1),若不做 spec 手术,服务端会选中 h2,而 transport 只会说
// HTTP/1.1,把上游的 HTTP/2 SETTINGS 帧当成 "malformed HTTP response" 断连
// (生产实测 api.moonshot.cn 全指纹断流)。服务端同时提供 h2/http/1.1 时,
// 指纹握手必须协商出 HTTP/1.1 并完成请求。
func TestUTLSFingerprintNegotiatesHTTP1Only(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	srv.StartTLS()
	defer srv.Close()

	baseTLS := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	fingerprints := []string{
		dto.TLSFingerprintChrome,
		dto.TLSFingerprintSafari,
		dto.TLSFingerprintIOS,
		dto.TLSFingerprintFirefox,
		dto.TLSFingerprintEdge,
		dto.TLSFingerprintAndroid,
		dto.TLSFingerprintRandomized,
	}
	for _, fp := range fingerprints {
		t.Run(fp, func(t *testing.T) {
			transport := newRelayHTTPTransport()
			transport.TLSClientConfig = baseTLS.Clone()
			applyUTLSFingerprint(transport, HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: 1, TLSFingerprint: fp}, nil)

			client := &http.Client{Transport: transport}
			resp, err := client.Get(srv.URL)
			require.NoError(t, err, "指纹 %s 对 h2 上游的请求必须成功", fp)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "HTTP/1.1", resp.Proto, "指纹 %s 必须只协商 HTTP/1.1", fp)
		})
	}
}
