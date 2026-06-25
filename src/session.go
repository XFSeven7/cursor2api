// 内存会话存储：user → Cursor chat_id 映射，支持 TTL 过期。
package main

import (
	"sync"
	"time"
)

// SessionStore 多轮对话 session 映射。
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
	ttl      time.Duration
}

type sessionEntry struct {
	chatID   string
	lastUsed time.Time
}

// NewSessionStore 创建 session 存储，ttlMs 为过期毫秒数。
func NewSessionStore(ttlMs int64) *SessionStore {
	return &SessionStore{
		sessions: make(map[string]sessionEntry),
		ttl:      time.Duration(ttlMs) * time.Millisecond,
	}
}

// GetChatID 获取已缓存的 Cursor chat_id。
func (s *SessionStore) GetChatID(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked()

	entry, ok := s.sessions[key]
	if !ok {
		return "", false
	}
	entry.lastUsed = time.Now()
	s.sessions[key] = entry
	return entry.chatID, true
}

// SetChatID 保存 user 与 chat_id 的映射。
func (s *SessionStore) SetChatID(key, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked()
	s.sessions[key] = sessionEntry{
		chatID:   chatID,
		lastUsed: time.Now(),
	}
}

func (s *SessionStore) purgeExpiredLocked() {
	if s.ttl <= 0 {
		return
	}
	now := time.Now()
	for key, entry := range s.sessions {
		if now.Sub(entry.lastUsed) > s.ttl {
			delete(s.sessions, key)
		}
	}
}
