package elicit

import (
	"context"
	"errors"
	"sync"
)

// ErrRequestNotFound is returned when a requested elicitation request is not found.
var ErrRequestNotFound = errors.New("request not found")

// MemoryStore implements the Store interface using in-memory maps.
type MemoryStore struct {
	mu        sync.RWMutex
	byID      map[string]*Request
	bySession map[string]*Request
}

// NewMemoryStore creates and returns a new MemoryStore instance.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:      make(map[string]*Request),
		bySession: make(map[string]*Request),
	}
}

// Create inserts a new request into both byID and bySession maps.
func (s *MemoryStore) Create(ctx context.Context, req *Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[req.ID] = req
	s.bySession[req.SessionID] = req
	return nil
}

// Get retrieves a request by its ID. Returns ErrRequestNotFound if not found.
func (s *MemoryStore) Get(ctx context.Context, id string) (*Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.byID[id]
	if !ok {
		return nil, ErrRequestNotFound
	}
	return req, nil
}

// GetBySessionID retrieves a request by its session ID. Returns ErrRequestNotFound if not found.
func (s *MemoryStore) GetBySessionID(ctx context.Context, sessionID string) (*Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.bySession[sessionID]
	if !ok {
		return nil, ErrRequestNotFound
	}
	return req, nil
}

// Delete removes a request by its ID. It only removes the session mapping if the ID matches
// the deleted request ID, preventing old deferred calls from deleting new sessions.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.byID[id]
	if !ok {
		return ErrRequestNotFound
	}
	delete(s.byID, id)
	if current, ok := s.bySession[req.SessionID]; ok && current.ID == id {
		delete(s.bySession, req.SessionID)
	}
	return nil
}
