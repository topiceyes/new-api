package audit

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ExtractPromptText 从各 relay 格式的请求 DTO 中提取可扫描的用户文本
// (消息文本段、system、instructions 等),不提取图片/音频等二进制内容。
// 未知类型回退为整请求 JSON 文本。结果按 maxBytes 截断。
func ExtractPromptText(req dto.Request, maxBytes int) string {
	if req == nil {
		return ""
	}
	var sb strings.Builder
	switch r := req.(type) {
	case *dto.GeneralOpenAIRequest:
		if r == nil {
			return ""
		}
		for i := range r.Messages {
			appendText(&sb, r.Messages[i].StringContent())
		}
		appendAnyText(&sb, r.Prompt)
		appendText(&sb, r.Instruction)
		appendAnyText(&sb, r.Input)
	case *dto.ClaudeRequest:
		if r == nil {
			return ""
		}
		appendText(&sb, r.Prompt)
		appendAnyText(&sb, r.System)
		for i := range r.Messages {
			msg := &r.Messages[i]
			if msg.IsStringContent() {
				appendText(&sb, msg.GetStringContent())
			} else {
				appendAnyText(&sb, msg.Content)
			}
		}
	case *dto.GeminiChatRequest:
		if r == nil {
			return ""
		}
		appendGeminiContents(&sb, r.Contents)
		if r.SystemInstructions != nil {
			appendGeminiContents(&sb, []dto.GeminiChatContent{*r.SystemInstructions})
		}
	case *dto.OpenAIResponsesRequest:
		if r == nil {
			return ""
		}
		appendRawMessageText(&sb, r.Instructions)
		appendRawMessageText(&sb, r.Input)
	default:
		// 回退:扫描整请求 JSON,文本内容天然包含在其中。
		if data, err := common.Marshal(req); err == nil {
			sb.Write(data)
		}
	}
	return truncateRunes(sb.String(), maxBytes)
}

func appendText(sb *strings.Builder, s string) {
	if s == "" {
		return
	}
	if sb.Len() > 0 {
		sb.WriteByte('\n')
	}
	sb.WriteString(s)
}

// appendAnyText 处理 any 类型的内容字段:string 直接取;数组/对象尝试提取
// 其中的 "text" 字段(多模态消息的文本段),其余结构忽略。
func appendAnyText(sb *strings.Builder, v any) {
	switch t := v.(type) {
	case nil:
		return
	case string:
		appendText(sb, t)
	case []any:
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					appendText(sb, s)
				}
			}
		}
	}
}

func appendGeminiContents(sb *strings.Builder, contents []dto.GeminiChatContent) {
	for _, content := range contents {
		for _, part := range content.Parts {
			appendText(sb, part.Text)
		}
	}
}

// appendRawMessageText 处理 json.RawMessage 字段:JSON 字符串取其值,
// 其他结构(数组/对象)直接拼入原始 JSON 文本一并扫描。
func appendRawMessageText(sb *strings.Builder, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var s string
	if err := common.Unmarshal(raw, &s); err == nil {
		appendText(sb, s)
		return
	}
	appendText(sb, string(raw))
}
