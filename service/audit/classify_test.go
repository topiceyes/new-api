package audit

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseClassifyResponse(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		batchSize int
		want      []classifyResult
	}{
		{
			name:      "clean json array",
			content:   `[{"idx":1,"category":"programming","skill_title":"SQL 优化"}]`,
			batchSize: 2,
			want:      []classifyResult{{Idx: 1, Category: "programming", SkillTitle: "SQL 优化"}},
		},
		{
			name: "tolerates prose around json",
			content: `好的,分类结果如下:
[{"idx":1,"category":"data","skill_title":"数据透视"},{"idx":2,"category":"chat","skill_title":"闲聊"}]
以上是结果。`,
			batchSize: 2,
			want: []classifyResult{
				{Idx: 1, Category: "data", SkillTitle: "数据透视"},
				{Idx: 2, Category: "chat", SkillTitle: "闲聊"},
			},
		},
		{
			name:      "drops out-of-range idx and invalid category",
			content:   `[{"idx":0,"category":"chat","skill_title":"a"},{"idx":5,"category":"chat","skill_title":"b"},{"idx":2,"category":"hacking","skill_title":"c"},{"idx":1,"category":"ops","skill_title":"发布变更"}]`,
			batchSize: 2,
			want:      []classifyResult{{Idx: 1, Category: "ops", SkillTitle: "发布变更"}},
		},
		{
			name:      "no json array returns nil",
			content:   `模型拒绝回答`,
			batchSize: 3,
			want:      nil,
		},
		{
			name:      "malformed json returns nil",
			content:   `[{"idx":1,"category":"chat",}]`,
			batchSize: 3,
			want:      nil,
		},
		{
			name:      "long skill title is truncated to 60 bytes",
			content:   `[{"idx":1,"category":"document","skill_title":"` + strings.Repeat("a", 80) + `"}]`,
			batchSize: 1,
			want:      []classifyResult{{Idx: 1, Category: "document", SkillTitle: strings.Repeat("a", 60)}},
		},
		{
			name:      "skill title whitespace trimmed, may be empty",
			content:   `[{"idx":1,"category":"other","skill_title":"  "}]`,
			batchSize: 1,
			want:      []classifyResult{{Idx: 1, Category: "other", SkillTitle: ""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClassifyResponse(tt.content, tt.batchSize)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildClassifyPrompt(t *testing.T) {
	prompt := buildClassifyPrompt([]string{"帮我优化 SQL", strings.Repeat("x", 600)})

	assert.Contains(t, prompt, "--- #1 ---\n帮我优化 SQL")
	assert.Contains(t, prompt, "--- #2 ---")
	// 第二条按 500 字节截断:截断后的样本不应完整出现。
	assert.NotContains(t, prompt, strings.Repeat("x", 600))
	assert.Contains(t, prompt, strings.Repeat("x", 500))
	for _, category := range classifyCategories {
		assert.Contains(t, prompt, category)
	}
}

func TestCallClassifyLLM(t *testing.T) {
	t.Run("returns content and sends bearer auth", func(t *testing.T) {
		var gotAuth string
		var gotModel string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			var body struct {
				Model string `json:"model"`
			}
			reqBody, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(reqBody, &body))
			gotModel = body.Model
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"[{\"idx\":1,\"category\":\"chat\"}]"}}]}`)
		}))
		defer server.Close()

		content, err := callClassifyLLM(server.URL, "sk-test-key", "gpt-4o-mini", "prompt")
		require.NoError(t, err)
		assert.Equal(t, `[{"idx":1,"category":"chat"}]`, content)
		assert.Equal(t, "Bearer sk-test-key", gotAuth)
		assert.Equal(t, "gpt-4o-mini", gotModel)
	})

	t.Run("non-200 surfaces status with truncated body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, strings.Repeat("x", 500))
		}))
		defer server.Close()

		_, err := callClassifyLLM(server.URL, "k", "m", "p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("empty choices is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[]}`)
		}))
		defer server.Close()

		_, err := callClassifyLLM(server.URL, "k", "m", "p")
		require.Error(t, err)
	})
}

func TestClassifyPendingEventsDisabled(t *testing.T) {
	// 总开关关闭时直接返回 0,不触碰渠道与数据库。
	withAuditSettings(t, func(s *system_setting.AuditSettings) {
		s.Enabled = false
		s.ClassifyEnabled = true
	})
	processed, classified, err := ClassifyPendingEvents()
	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 0, classified)
}

func TestClassifyPendingEventsMissingChannelDegrades(t *testing.T) {
	// 分类开启但 channel/model 未配置:记日志并安静跳过,不报错。
	withAuditSettings(t, func(s *system_setting.AuditSettings) {
		s.Enabled = true
		s.ClassifyEnabled = true
		s.ClassifyChannelId = 0
		s.ClassifyModel = ""
	})
	processed, classified, err := ClassifyPendingEvents()
	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 0, classified)
}
