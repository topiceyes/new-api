package audit

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// LLM 分类(二期②):定时任务取带 prompt 原文的未分类审计事件,经配置的
// OpenAI 兼容渠道分类,结果写回 audit_events.category 并按 skill 标题归并
// 候选(skill_candidates),管理员审核后进入 skill 库。

// 分类类别枚举(与分类 prompt 中给模型的约束一致)。
var classifyCategories = []string{"programming", "document", "data", "ops", "chat", "other"}

// classifyPromptSampleMaxRunes 每条送分类的 prompt 摘要截断长度。
const classifyPromptSampleMaxBytes = 500

// classifyHTTPClient 分类请求的 HTTP 客户端(测试 seam:httptest 替换)。
var classifyHTTPClient = &http.Client{Timeout: 60 * time.Second}

// classifyResult 单条事件的模型分类输出。
type classifyResult struct {
	Idx        int    `json:"idx"`
	Category   string `json:"category"`
	SkillTitle string `json:"skill_title"`
}

func isValidClassifyCategory(category string) bool {
	for _, c := range classifyCategories {
		if c == category {
			return true
		}
	}
	return false
}

// classifyEndpoint 从配置渠道解析分类调用地址与凭据。仅支持 OpenAI 兼容渠道
// (直连 {base_url}/v1/chat/completions);渠道缺失/无 key 返回错误,本轮安静跳过。
func classifyEndpoint(settings *system_setting.AuditSettings) (url string, key string, err error) {
	if settings.ClassifyChannelId == 0 || settings.ClassifyModel == "" {
		return "", "", fmt.Errorf("classify channel/model not configured")
	}
	channel, err := model.GetChannelById(settings.ClassifyChannelId, true)
	if err != nil {
		return "", "", fmt.Errorf("classify channel %d not found: %w", settings.ClassifyChannelId, err)
	}
	keys := channel.GetKeys()
	if len(keys) == 0 || keys[0] == "" {
		return "", "", fmt.Errorf("classify channel %d has no key", settings.ClassifyChannelId)
	}
	return strings.TrimSuffix(channel.GetBaseURL(), "/") + "/v1/chat/completions", keys[0], nil
}

// buildClassifyPrompt 组装分类请求:编号列出 prompt 摘要,要求输出 JSON 数组。
func buildClassifyPrompt(prompts []string) string {
	var b strings.Builder
	b.WriteString("你是企业内部的 AI 使用行为分析助手。下面是若干条员工发给 AI 的 prompt(已按 500 字符截断)。\n")
	b.WriteString("请对每条输出分类与 skill 标题。分类只能是: " + strings.Join(classifyCategories, ", ") + "。\n")
	b.WriteString("skill_title 是对该 prompt 所代表的可复用技能的简短命名(10 字以内,同类 prompt 必须给出相同标题,便于归并)。\n")
	b.WriteString("只输出 JSON 数组,不要输出任何其他文字:[{\"idx\":1,\"category\":\"programming\",\"skill_title\":\"SQL 优化\"}]\n\n")
	for i, p := range prompts {
		fmt.Fprintf(&b, "--- #%d ---\n%s\n", i+1, truncateRunes(p, classifyPromptSampleMaxBytes))
	}
	return b.String()
}

// callClassifyLLM 直连 OpenAI 兼容渠道执行分类,返回模型的 content 文本。
func callClassifyLLM(url string, key string, modelName string, prompt string) (string, error) {
	reqBody, err := common.Marshal(map[string]any{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := classifyHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("classify channel returned %d: %s", resp.StatusCode, truncateRunes(string(body), 200))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := common.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("invalid classify channel response: %w", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("classify channel returned empty content")
	}
	return parsed.Choices[0].Message.Content, nil
}

// parseClassifyResponse 从模型输出中提取 JSON 数组并校验。
// 容错:截取首个 [ 到末个 ],忽略缺 idx/越界 idx/非法 category 的条目;同 idx 重复只留首条。
func parseClassifyResponse(content string, batchSize int) []classifyResult {
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end <= start {
		return nil
	}
	var raw []classifyResult
	if err := common.UnmarshalJsonStr(content[start:end+1], &raw); err != nil {
		return nil
	}
	seen := make(map[int]bool, len(raw))
	results := make([]classifyResult, 0, len(raw))
	for _, r := range raw {
		if r.Idx < 1 || r.Idx > batchSize {
			continue
		}
		if !isValidClassifyCategory(r.Category) {
			continue
		}
		if seen[r.Idx] {
			continue
		}
		seen[r.Idx] = true
		r.SkillTitle = strings.TrimSpace(truncateRunes(r.SkillTitle, 60))
		results = append(results, r)
	}
	return results
}

// ClassifyPendingEvents 执行一轮分类,返回处理/成功条数。供 SystemTask 调用。
// 任何一步失败只记日志并跳过该轮,不影响其他功能(安静降级)。
func ClassifyPendingEvents() (processed int, classified int, err error) {
	settings := auditSettingsSnapshot()
	if !settings.Enabled || !settings.ClassifyEnabled {
		return 0, 0, nil
	}
	batchSize := settings.ClassifyBatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	url, key, err := classifyEndpoint(settings)
	if err != nil {
		common.SysError("audit classify: " + err.Error())
		return 0, 0, nil
	}

	events, err := model.GetUnclassifiedPromptEvents(batchSize)
	if err != nil {
		return 0, 0, err
	}
	if len(events) == 0 {
		return 0, 0, nil
	}
	processed = len(events)

	prompts := make([]string, len(events))
	for i, e := range events {
		prompts[i] = e.Prompt
	}
	content, err := callClassifyLLM(url, key, settings.ClassifyModel, buildClassifyPrompt(prompts))
	if err != nil {
		common.SysError("audit classify: LLM call failed: " + err.Error())
		return processed, 0, nil
	}

	results := parseClassifyResponse(content, len(events))
	now := time.Now().Unix()
	classifiedIdx := make(map[int]bool, len(results))
	for _, r := range results {
		classifiedIdx[r.Idx] = true
		event := events[r.Idx-1]
		if err := model.UpdateAuditEventCategory(event.Id, r.Category); err != nil {
			common.SysError(fmt.Sprintf("audit classify: update event %d category failed: %s", event.Id, err.Error()))
			continue
		}
		classified++
		if r.SkillTitle == "" {
			continue
		}
		sample := truncateRunes(event.Prompt, classifyPromptSampleMaxBytes)
		if _, err := model.UpsertSkillCandidate(r.SkillTitle, r.Category, sample, event.UserId, now); err != nil {
			common.SysError("audit classify: upsert candidate failed: " + err.Error())
		}
	}
	// LLM 调用成功但模型没给出合法结果的条目(拒答/漏项),标 classify_failed
	// 防止队头阻塞:这批事件每轮都被取走重新计费,后面的新事件永远排不上。
	// 暂时性失败(网络/非 200)在上面已提前返回,不会走到这里。
	for i, event := range events {
		if classifiedIdx[i+1] {
			continue
		}
		if err := model.UpdateAuditEventCategory(event.Id, "classify_failed"); err != nil {
			common.SysError(fmt.Sprintf("audit classify: mark event %d failed failed: %s", event.Id, err.Error()))
		}
	}
	return processed, classified, nil
}
