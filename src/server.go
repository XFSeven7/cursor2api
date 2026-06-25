// HTTP 路由与请求处理：健康检查、模型列表、对话接口。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Server API 服务主体。
type Server struct {
	cfg     Config
	agent   *AgentRunner
	session *SessionStore
}

// NewServer 创建服务实例。
func NewServer(cfg Config) *Server {
	return &Server{
		cfg:     cfg,
		agent:   NewAgentRunner(cfg),
		session: NewSessionStore(cfg.SessionTTLMs),
	}
}

// Handler 返回带 CORS 的 HTTP 处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	return withPrivateNetworkCORS(withCORS(mux))
}

// Warmup 启动时预热（检查 agent 登录状态）。
func (s *Server) Warmup(ctx context.Context) {
	_, _ = s.agent.CheckLogin(ctx)
}

func (s *Server) authorize(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	return strings.TrimSpace(auth[len(prefix):]) == s.cfg.APIKey
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	loggedIn, detail := s.agent.CheckLogin(r.Context())
	status := "ok"
	agentStatus := "logged_in"
	if !loggedIn {
		agentStatus = "not_logged_in"
	}
	pool := make([]string, 0, len(autoModelPool))
	for _, m := range autoModelPool {
		pool = append(pool, m.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    status,
		"agent":     agentStatus,
		"detail":    detail,
		"model":     autoModelID,
		"auto_pool": pool,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, "invalid api key", "invalid_request_error")
		return
	}

	created := time.Now().Unix()
	writeJSON(w, http.StatusOK, modelsListResponse{
		Object: "list",
		Data: []modelItem{{
			ID:      autoModelID,
			Object:  "model",
			Created: created,
			OwnedBy: "cursor",
		}},
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, "invalid api key", "invalid_request_error")
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body", "invalid_request_error")
		return
	}

	loggedIn, _ := s.agent.CheckLogin(r.Context())
	if !loggedIn {
		writeError(w, http.StatusServiceUnavailable, "agent not logged in, run: agent login", "service_unavailable")
		return
	}

	prompt, err := extractPrompt(req.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	model, err := resolveModel(req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	key := sessionKey(req)
	chatID, _ := s.session.GetChatID(key)

	if req.Stream {
		s.handleStream(w, r, prompt, chatID, model, key)
		return
	}
	s.handleNonStream(w, r, prompt, chatID, model, key)
}

func (s *Server) handleNonStream(w http.ResponseWriter, r *http.Request, prompt, chatID, model, sessionKey string) {
	result, err := s.agent.RunOnce(r.Context(), prompt, chatID, model)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			writeError(w, http.StatusGatewayTimeout, "agent request timed out", "timeout_error")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error(), "api_error")
		return
	}

	s.persistSession(sessionKey, result.SessionID)
	if result.SessionID != "" {
		w.Header().Set("X-Cursor-Session-Id", result.SessionID)
	}

	id := newCompletionID()
	writeJSON(w, http.StatusOK, buildCompletion(id, model, result.Text, result.Usage))
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, prompt, chatID, model, sessionKey string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id := newCompletionID()
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported", "server_error")
		return
	}

	result, err := s.agent.RunStream(r.Context(), prompt, chatID, model, func(delta string) error {
		return writeSSE(w, buildChunk(id, model, delta, false))
	})
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			writeError(w, http.StatusGatewayTimeout, "agent request timed out", "timeout_error")
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(errorResponse{
			Error: openAIError{Message: err.Error(), Type: "api_error"},
		}))
		flusher.Flush()
		return
	}

	if result.SessionID != "" {
		w.Header().Set("X-Cursor-Session-Id", result.SessionID)
	}

	s.persistSession(sessionKey, result.SessionID)

	_ = writeSSE(w, buildChunk(id, model, "", true))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// persistSession 将 agent 返回的 session_id 写入内存映射，供多轮 --resume 使用。
func (s *Server) persistSession(key, sessionID string) {
	if key == "" || sessionID == "" {
		return
	}
	s.session.SetChatID(key, sessionID)
}
