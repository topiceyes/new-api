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
			// HelloRandomized 每次握手随机生成扩展/曲线组合,偶尔会产生对端
			// 不接受的 ClientHello(如 Go 服务端拒绝随机曲线 ID)——这是 uTLS
			// 随机化模式的固有特性,真实上游(nginx/CF)容忍度更高。对随机指纹
			// 允许重试,预设指纹保持单次必成功。
			attempts := 1
			if fp == dto.TLSFingerprintRandomized {
				attempts = 5
			}
			var resp *http.Response
			var err error
			for i := 0; i < attempts; i++ {
				resp, err = client.Get(srv.URL)
				if err == nil {
					break
				}
			}
			require.NoError(t, err, "指纹 %s 对 h2 上游的请求必须成功", fp)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "HTTP/1.1", resp.Proto, "指纹 %s 必须只协商 HTTP/1.1", fp)
		})
	}
}

// TestUTLSFingerprintRepeatedHandshakes 回归:presetSpec 不能共享——ApplyPreset
// 让 uconn 持有 spec 扩展指针并就地改写,共享 spec 时第二次握手必炸
// (tls: internal error,生产实测:首请求成功,后续全部失败)。同一 transport
// 的 DialTLSContext 连续 3 次握手必须全部成功。
func TestUTLSFingerprintRepeatedHandshakes(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := newRelayHTTPTransport()
	transport.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	applyUTLSFingerprint(transport, HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: 1, TLSFingerprint: dto.TLSFingerprintChrome}, nil)
	require.NotNil(t, transport.DialTLSContext)

	for i := 0; i < 3; i++ {
		conn, err := transport.DialTLSContext(t.Context(), "tcp", srv.Listener.Addr().String())
		require.NoError(t, err, "第 %d 次握手必须成功", i)
		require.NoError(t, conn.Close())
	}
}
