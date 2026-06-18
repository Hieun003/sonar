package elicit

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// QuestionType defines the type of selection question.
type QuestionType string

const (
	// SingleSelect represents a question where only one option can be chosen.
	SingleSelect QuestionType = "single_select"
	// MultiSelect represents a question where multiple options can be chosen.
	MultiSelect QuestionType = "multi_select"
	// RankPriority represents a question where options are ranked/prioritized.
	RankPriority QuestionType = "rank_priorities"
)

// Question represents a query with options for selection.
type Question struct {
	Question string       `json:"question"`
	Type     QuestionType `json:"type"`
	Options  []string     `json:"options"`
}

// Answer represents the response to a specific question.
type Answer struct {
	Question string   `json:"question"`
	Selected []string `json:"selected"`
}

// Result represents the collected answers for a request.
type Result struct {
	Answers []Answer `json:"answers"`
}

// Request represents a pending elicitation request.
type Request struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Questions []Question `json:"questions"`
	CreatedAt time.Time  `json:"created_at"`
}

// Store defines the storage interface for requests.
type Store interface {
	// Create saves a request in the store.
	Create(ctx context.Context, req *Request) error
	// Get retrieves a request by its ID.
	Get(ctx context.Context, id string) (*Request, error)
	// GetBySessionID retrieves a request by its session ID.
	GetBySessionID(ctx context.Context, sessionID string) (*Request, error)
	// Delete removes a request by its ID.
	Delete(ctx context.Context, id string) error
}

// Notifier defines the interface for notifying users of new elicitation requests.
type Notifier interface {
	// Notify sends a notification for a request.
	Notify(ctx context.Context, req *Request) error
}

// Logger defines the interface for logging warnings. It is compatible with slog.Logger.
type Logger interface {
	// Warn logs a warning message with arguments.
	Warn(msg string, args ...any)
}

// resultOrErr wraps the outcome of an elicitation request.
type resultOrErr struct {
	result Result
	err    error
}

// Manager manages elicitation sessions, orchestrating request lifecycle and concurrency.
type Manager struct {
	store    Store
	notifier Notifier
	timeout  time.Duration
	logger   Logger
	mu       sync.Mutex
	pending  map[string]chan resultOrErr
	// sessionLocks is a map of per-sessionID mutexes.
	// Map này không được evict để tránh race condition. Với hệ thống lớn, cân nhắc sharded mutex pool.
	sessionLocks sync.Map
}

// NewManager creates a new Manager instance.
func NewManager(store Store, notifier Notifier, timeout time.Duration, logger Logger) *Manager {
	return &Manager{
		store:    store,
		notifier: notifier,
		timeout:  timeout,
		logger:   logger,
		pending:  make(map[string]chan resultOrErr),
	}
}

// ErrTimeout is returned when an elicitation request times out.
var ErrTimeout = errors.New("elicitation timeout")

// ErrNotFoundOrResolved is returned when an elicitation request is not found or has already been resolved.
var ErrNotFoundOrResolved = errors.New("not found or already resolved")

// ErrSessionCancelled is returned when a session is cancelled by a new request or administrative action.
var ErrSessionCancelled = errors.New("session cancelled by new request or administrative action")

// Elicit triggers the elicitation workflow for a session.
// It blocks until a result is received, context is cancelled, or timeout occurs.
func (m *Manager) Elicit(ctx context.Context, sessionID string, questions []Question) (Result, error) {
	// 1. Lấy per-session mutex từ `sessionLocks` bằng `LoadOrStore`, lock nó
	sLockInterface, _ := m.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	sLock := sLockInterface.(*sync.Mutex)
	sLock.Lock()

	// 2. Gọi `cancelSessionLocked(ctx, sessionID)` để hủy request cũ nếu có
	m.cancelSessionLocked(ctx, sessionID)

	// 3. Tạo `Request` mới với `uuid.New().String()` làm ID
	reqID := uuid.New().String()
	req := &Request{
		ID:        reqID,
		SessionID: sessionID,
		Questions: questions,
		CreatedAt: time.Now(),
	}

	// 4. Gọi `store.Create()` — nếu lỗi thì unlock sLock và return error
	if err := m.store.Create(ctx, req); err != nil {
		sLock.Unlock()
		return Result{}, err
	}

	// 5. Tạo `resCh := make(chan resultOrErr, 1)`, lock `m.mu`, ghi vào `m.pending[reqID]`, unlock `m.mu`
	resCh := make(chan resultOrErr, 1)
	m.mu.Lock()
	if m.pending == nil {
		m.pending = make(map[string]chan resultOrErr)
	}
	m.pending[reqID] = resCh
	m.mu.Unlock()

	// 6. Unlock `sLock` — PHẢI unlock trước khi block
	sLock.Unlock()

	// 7. Đăng ký `defer` dọn dẹp: xóa `m.pending[reqID]` và gọi `store.Delete(reqID)`
	// KHÔNG delete `sessionLocks` entry trong defer
	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
		_ = m.store.Delete(context.Background(), reqID)
	}()

	// 8. Gọi `notifier.Notify()` — nếu lỗi thì log warning, KHÔNG return, tiếp tục block
	if err := m.notifier.Notify(ctx, req); err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to notify elicitation request", "request_id", reqID, "error", err)
		}
	}

	// 9. Tạo `timer := time.NewTimer(m.timeout)`, defer `timer.Stop()`
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()

	// 10. `select` trên 3 case: `resCh`, `timer.C`, `ctx.Done()`
	select {
	case res := <-resCh:
		return res.result, res.err
	case <-timer.C:
		return Result{}, ErrTimeout
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// Resolve completes a pending elicitation request with answers.
func (m *Manager) Resolve(ctx context.Context, requestID string, result Result) error {
	// Lock m.mu, lấy resCh từ m.pending[requestID], unlock ngay
	m.mu.Lock()
	resCh, ok := m.pending[requestID]
	m.mu.Unlock()

	// Nếu không tìm thấy: return error "not found or already resolved"
	if !ok {
		return ErrNotFoundOrResolved
	}

	// Gửi vào resCh với select gồm 2 case: gửi thành công hoặc ctx.Done()
	// KHÔNG tự xóa pending hay store — để defer của Elicit() lo
	select {
	case resCh <- resultOrErr{result: result}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// cancelSessionLocked cancels any existing pending request for the given session ID.
//
// Store.GetBySessionID tuyệt đối KHÔNG được gọi ngược lại các method của Manager để tránh deadlock reentrancy.
func (m *Manager) cancelSessionLocked(ctx context.Context, sessionID string) {
	// Gọi store.GetBySessionID(ctx, sessionID) để tìm request cũ
	req, err := m.store.GetBySessionID(ctx, sessionID)
	// Nếu không có: return sớm
	if err != nil || req == nil {
		return
	}

	// Lock m.mu, lấy resCh từ m.pending[req.ID], unlock ngay
	m.mu.Lock()
	resCh, ok := m.pending[req.ID]
	m.mu.Unlock()

	if !ok {
		return
	}

	// Gửi resultOrErr{err: errors.New("session cancelled...")} vào resCh bằng non-blocking select (có default)
	// KHÔNG gọi store.Delete ở đây — để defer của Elicit() cũ tự dọn
	select {
	case resCh <- resultOrErr{err: ErrSessionCancelled}:
	default:
		// If sending blocks, the channel is already resolved or cancelled.
	}
}

// GetPending retrieves the pending elicitation request for the specified session ID.
func (m *Manager) GetPending(ctx context.Context, sessionID string) (*Request, error) {
	return m.store.GetBySessionID(ctx, sessionID)
}
