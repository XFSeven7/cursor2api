// OpenAI Chat Completions 请求/响应格式转换。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChatCompletionRequest OpenAI 兼容对话请求体。
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	User     string        `json:"user"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAIError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

type errorResponse struct {
	Error openAIError `json:"error"`
}

type modelsListResponse struct {
	Object string      `json:"object"`
	Data   []modelItem `json:"data"`
}

type modelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type chatCompletion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      *msg    `json:"message,omitempty"`
	Delta        *delta  `json:"delta,omitempty"`
	FinishReason *string `json:"finish_reason"`
}

type msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type delta struct {
	Content string `json:"content,omitempty"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message, errType string) {
	writeJSON(w, status, errorResponse{
		Error: openAIError{
			Message: message,
			Type:    errType,
		},
	})
}

// extractPrompt 从 messages 提取 system 前缀 + 最后一条 user 消息。
func extractPrompt(messages []ChatMessage) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("messages is required")
	}

	var systemParts []string
	var lastUser string

	for _, m := range messages {
		content := messageContent(m.Content)
		switch m.Role {
		case "system":
			if content != "" {
				systemParts = append(systemParts, content)
			}
		case "user":
			if content != "" {
				lastUser = content
			}
		}
	}

	if lastUser == "" {
		return "", fmt.Errorf("at least one user message is required")
	}

	if len(systemParts) == 0 {
		return lastUser, nil
	}
	return strings.Join(systemParts, "\n\n") + "\n\n" + lastUser, nil
}

func messageContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(content)
	}
}

// sessionKey 生成会话键：优先用 user 字段，否则按 messages 哈希。
func sessionKey(req ChatCompletionRequest) string {
	if req.User != "" {
		return req.User
	}
	raw, _ := json.Marshal(req.Messages)
	sum := sha256.Sum256(raw)
	return "anon:" + hex.EncodeToString(sum[:8])
}

func newCompletionID() string {
	return "chatcmpl-" + hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:24]
}

func buildCompletion(id, model, text string, usageMap map[string]any) chatCompletion {
	finish := "stop"
	resp := chatCompletion{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []choice{{
			Index: 0,
			Message: &msg{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: &finish,
		}},
	}
	if u := mapUsage(usageMap); u != nil {
		resp.Usage = u
	}
	return resp
}

func buildChunk(id, model, deltaText string, done bool) chatCompletion {
	chunk := chatCompletion{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []choice{{
			Index: 0,
			Delta: &delta{},
		}},
	}
	if done {
		reason := "stop"
		chunk.Choices[0].FinishReason = &reason
	} else if deltaText != "" {
		chunk.Choices[0].Delta.Content = deltaText
	}
	return chunk
}

func mapUsage(raw map[string]any) *usage {
	if raw == nil {
		return nil
	}
	in := asInt(raw["inputTokens"])
	out := asInt(raw["outputTokens"])
	if in == 0 && out == 0 {
		return nil
	}
	return &usage{
		PromptTokens:     in,
		CompletionTokens: out,
		TotalTokens:      in + out,
	}
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

// writeSSE 输出 OpenAI 兼容的 SSE 数据行。
func writeSSE(w http.ResponseWriter, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
