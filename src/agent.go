// 封装 Cursor agent CLI 子进程调用（非流式 / 流式）。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// AgentRunner 负责调用本机 agent 命令。
type AgentRunner struct {
	cfg Config

	// 登录状态缓存，避免每次请求都执行 agent status。
	loginMu      sync.RWMutex
	loginCached  bool
	loginDetail  string
	loginChecked time.Time
}

// AgentResult agent 执行结果。
type AgentResult struct {
	Text      string
	SessionID string
	Usage     map[string]any
}

// streamEvent agent --output-format stream-json 的 NDJSON 行结构。
type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Result  string `json:"result"`
	Message *struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
	IsError   bool           `json:"is_error"`
	Usage     map[string]any `json:"usage"`
	SessionID string         `json:"session_id"`
}

// NewAgentRunner 创建 agent 执行器。
func NewAgentRunner(cfg Config) *AgentRunner {
	return &AgentRunner{cfg: cfg}
}

// CheckLogin 检查 agent 是否已登录（带 TTL 缓存）。
func (r *AgentRunner) CheckLogin(ctx context.Context) (bool, string) {
	ttl := time.Duration(r.cfg.LoginCacheMs) * time.Millisecond
	if ttl > 0 {
		r.loginMu.RLock()
		if !r.loginChecked.IsZero() && time.Since(r.loginChecked) < ttl {
			ok, detail := r.loginCached, r.loginDetail
			r.loginMu.RUnlock()
			return ok, detail
		}
		r.loginMu.RUnlock()
	}

	ok, detail := r.checkLoginNow(ctx)

	r.loginMu.Lock()
	r.loginCached = ok
	r.loginDetail = detail
	r.loginChecked = time.Now()
	r.loginMu.Unlock()

	return ok, detail
}

func (r *AgentRunner) checkLoginNow(ctx context.Context) (bool, string) {
	out, err := r.run(ctx, r.cfg.AgentPath, "status")
	if err != nil {
		return false, ""
	}
	text := strings.TrimSpace(out)
	if strings.Contains(text, "Logged in") || strings.Contains(text, "✓") {
		return true, text
	}
	return false, text
}

// InvalidateLoginCache 清除登录缓存。
func (r *AgentRunner) InvalidateLoginCache() {
	r.loginMu.Lock()
	r.loginChecked = time.Time{}
	r.loginMu.Unlock()
}

func (r *AgentRunner) ListModels(ctx context.Context) ([]ModelInfo, error) {
	out, err := r.run(ctx, r.cfg.AgentPath, "models")
	if err != nil {
		return nil, err
	}
	return parseModelsOutput(out), nil
}

// RunOnce 非流式执行，等待完整 JSON 结果。
func (r *AgentRunner) RunOnce(ctx context.Context, prompt, chatID, model string) (*AgentResult, error) {
	args := r.baseArgs(prompt, chatID, model)
	args = append(args, "--output-format", "json")

	out, err := r.run(ctx, r.cfg.AgentPath, args...)
	if err != nil {
		return nil, err
	}

	var event streamEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &event); err != nil {
		return nil, fmt.Errorf("parse agent json: %w", err)
	}
	if event.IsError || event.Subtype == "error" {
		return nil, fmt.Errorf("agent error: %s", event.Result)
	}

	return &AgentResult{
		Text:      event.Result,
		SessionID: event.SessionID,
		Usage:     event.Usage,
	}, nil
}

// RunStream 流式执行，逐段回调文本增量。
func (r *AgentRunner) RunStream(ctx context.Context, prompt, chatID, model string, onDelta func(string) error) (*AgentResult, error) {
	args := r.baseArgs(prompt, chatID, model)
	args = append(args,
		"--output-format", "stream-json",
		"--stream-partial-output",
	)

	timeout := time.Duration(r.cfg.RequestTimeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, r.cfg.AgentPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	var final streamEvent
	var sentLen int // 已发送字符数，用于去重累积文本
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		switch event.Type {
		case "assistant":
			if event.Message == nil {
				continue
			}
			for _, block := range event.Message.Content {
				if block.Type != "text" || block.Text == "" {
					continue
				}
				text := block.Text
				if len(text) <= sentLen {
					continue
				}
				delta := text[sentLen:]
				sentLen = len(text)
				if err := onDelta(delta); err != nil {
					_ = cmd.Process.Kill()
					return nil, err
				}
			}
		case "result":
			final = event
		}
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("agent failed: %w: %s", err, stderr.String())
		}
		return nil, fmt.Errorf("agent failed: %w", err)
	}
	if final.IsError {
		return nil, fmt.Errorf("agent error: %s", final.Result)
	}

	return &AgentResult{
		Text:      final.Result,
		SessionID: final.SessionID,
		Usage:     final.Usage,
	}, nil
}

// baseArgs 构造 agent 命令参数；chatID 为空时不传 --resume（首轮对话）。
func (r *AgentRunner) baseArgs(prompt, chatID, model string) []string {
	if model == "" {
		model = autoModelID
	}
	args := []string{
		"-p", prompt,
		"--mode", r.cfg.AgentMode,
		"--model", model,
		"--trust",
		"--workspace", r.cfg.Workspace,
	}
	if chatID != "" {
		args = append(args, "--resume", chatID)
	}
	return args
}

func (r *AgentRunner) run(ctx context.Context, name string, args ...string) (string, error) {
	timeout := time.Duration(r.cfg.RequestTimeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

func parseModelsOutput(out string) []ModelInfo {
	var models []ModelInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Available models") {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if id == "" {
			continue
		}
		models = append(models, ModelInfo{ID: id, Name: name})
	}
	return models
}
