package audit

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractPromptTextGeneralOpenAI 覆盖字符串消息、多模态文本段与 completion 风格字段。
func TestExtractPromptTextGeneralOpenAI(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "识别这张图片"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
			}},
		},
	}
	got := ExtractPromptText(req, 1<<20)
	assert.Contains(t, got, "你是助手")
	assert.Contains(t, got, "识别这张图片")
	assert.NotContains(t, got, "image_url")

	legacy := &dto.GeneralOpenAIRequest{
		Prompt:      "补全这段话",
		Instruction: "保持简洁",
	}
	got = ExtractPromptText(legacy, 1<<20)
	assert.Contains(t, got, "补全这段话")
	assert.Contains(t, got, "保持简洁")
}

func TestExtractPromptTextClaude(t *testing.T) {
	req := &dto.ClaudeRequest{
		System: "系统提示",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "你好"},
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "分析这个文件"},
			}},
		},
	}
	got := ExtractPromptText(req, 1<<20)
	assert.Contains(t, got, "系统提示")
	assert.Contains(t, got, "你好")
	assert.Contains(t, got, "分析这个文件")

	// 旧版 prompt 字段
	legacy := &dto.ClaudeRequest{Prompt: "\n\nHuman: 老格式"}
	assert.Contains(t, ExtractPromptText(legacy, 1<<20), "老格式")
}

func TestExtractPromptTextGemini(t *testing.T) {
	req := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: []dto.GeminiPart{{Text: "总结一下"}, {InlineData: &dto.GeminiInlineData{}}}},
		},
		SystemInstructions: &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: "用中文回答"}},
		},
	}
	got := ExtractPromptText(req, 1<<20)
	assert.Contains(t, got, "总结一下")
	assert.Contains(t, got, "用中文回答")
}

func TestExtractPromptTextResponses(t *testing.T) {
	// instructions 为 JSON 字符串,input 为结构化数组时拼入原始 JSON 一并扫描
	req := &dto.OpenAIResponsesRequest{
		Instructions: []byte(`"直接回答"`),
		Input:        []byte(`[{"role":"user","content":[{"type":"input_text","text":"敏感词甲"}]}]`),
	}
	got := ExtractPromptText(req, 1<<20)
	assert.Contains(t, got, "直接回答")
	assert.Contains(t, got, "敏感词甲")
}

// TestExtractPromptTextFallback 未知 DTO 类型回退为整请求 JSON 扫描。
func TestExtractPromptTextFallback(t *testing.T) {
	type customRequest struct {
		dto.BaseRequest
		Query string `json:"query"`
	}
	got := ExtractPromptText(&customRequest{Query: "特殊格式的查询内容"}, 1<<20)
	assert.Contains(t, got, "特殊格式的查询内容")
}

// TestExtractPromptTextTruncation 结果按字节上限截断且不切断多字节字符。
func TestExtractPromptTextTruncation(t *testing.T) {
	long := strings.Repeat("汉", 100)
	req := &dto.ClaudeRequest{Prompt: long}
	got := ExtractPromptText(req, 10)
	require.LessOrEqual(t, len(got), 10)
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, strings.Repeat("汉", 3), got)
}

func TestExtractPromptTextNil(t *testing.T) {
	assert.Equal(t, "", ExtractPromptText(nil, 1024))
	var req *dto.ClaudeRequest
	assert.Equal(t, "", ExtractPromptText(req, 1024))
}
