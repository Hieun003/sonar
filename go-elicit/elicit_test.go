package elicit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockNotifier struct {
	notifyFunc func(ctx context.Context, req *Request) error
}

func (n *mockNotifier) Notify(ctx context.Context, req *Request) error {
	if n.notifyFunc != nil {
		return n.notifyFunc(ctx, req)
	}
	return nil
}

type mockLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func TestElicitAndResolve(t *testing.T) {
	store := NewMemoryStore()
	notifier := &mockNotifier{}
	logger := &mockLogger{}
	manager := NewManager(store, notifier, 100*time.Millisecond, logger)

	questions := []Question{
		{Question: "Choose one", Type: SingleSelect, Options: []string{"A", "B"}},
	}

	ctx := context.Background()
	sessionID := "session-1"

	type elicitResult struct {
		res Result
		err error
	}
	resCh := make(chan elicitResult, 1)

	notifier.notifyFunc = func(ctx context.Context, req *Request) error {
		go func() {
			ans := Result{
				Answers: []Answer{
					{Question: "Choose one", Selected: []string{"A"}},
				},
			}
			err := manager.Resolve(context.Background(), req.ID, ans)
			if err != nil {
				t.Errorf("failed to resolve request: %v", err)
			}
		}()
		return nil
	}

	go func() {
		res, err := manager.Elicit(ctx, sessionID, questions)
		resCh <- elicitResult{res, err}
	}()

	select {
	case out := <-resCh:
		if out.err != nil {
			t.Fatalf("unexpected error: %v", out.err)
		}
		if len(out.res.Answers) != 1 || out.res.Answers[0].Selected[0] != "A" {
			t.Fatalf("unexpected result: %+v", out.res)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("test timed out")
	}

	reqs, _ := store.GetBySessionID(context.Background(), sessionID)
	if reqs != nil {
		t.Error("request was not cleaned up from store")
	}
}

func TestElicitTimeout(t *testing.T) {
	store := NewMemoryStore()
	notifier := &mockNotifier{}
	logger := &mockLogger{}
	manager := NewManager(store, notifier, 10*time.Millisecond, logger)

	questions := []Question{
		{Question: "Choose one", Type: SingleSelect, Options: []string{"A", "B"}},
	}

	_, err := manager.Elicit(context.Background(), "session-2", questions)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}

	_, err = store.GetBySessionID(context.Background(), "session-2")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Errorf("expected ErrRequestNotFound, got %v", err)
	}
}

func TestElicitContextCancelled(t *testing.T) {
	store := NewMemoryStore()
	notifier := &mockNotifier{}
	logger := &mockLogger{}
	manager := NewManager(store, notifier, 1*time.Second, logger)

	ctx, cancel := context.WithCancel(context.Background())
	questions := []Question{
		{Question: "Choose one", Type: SingleSelect, Options: []string{"A", "B"}},
	}

	notifier.notifyFunc = func(ctx context.Context, req *Request) error {
		cancel()
		return nil
	}

	_, err := manager.Elicit(ctx, "session-3", questions)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	_, err = store.GetBySessionID(context.Background(), "session-3")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Errorf("expected ErrRequestNotFound, got %v", err)
	}
}

func TestSessionCancellation(t *testing.T) {
	store := NewMemoryStore()
	notifier := &mockNotifier{}
	logger := &mockLogger{}
	manager := NewManager(store, notifier, 1*time.Second, logger)

	questions := []Question{
		{Question: "Choose one", Type: SingleSelect, Options: []string{"A", "B"}},
	}

	req1Created := make(chan struct{})
	notifier.notifyFunc = func(ctx context.Context, req *Request) error {
		close(req1Created)
		return nil
	}

	errCh1 := make(chan error, 1)
	go func() {
		_, err := manager.Elicit(context.Background(), "session-4", questions)
		errCh1 <- err
	}()

	<-req1Created

	errCh2 := make(chan error, 1)
	notifier.notifyFunc = func(ctx context.Context, req *Request) error {
		go func() {
			ans := Result{Answers: []Answer{{Question: "Choose one", Selected: []string{"B"}}}}
			_ = manager.Resolve(context.Background(), req.ID, ans)
		}()
		return nil
	}

	go func() {
		_, err := manager.Elicit(context.Background(), "session-4", questions)
		errCh2 <- err
	}()

	err1 := <-errCh1
	if !errors.Is(err1, ErrSessionCancelled) {
		t.Errorf("expected ErrSessionCancelled error, got %v", err1)
	}

	err2 := <-errCh2
	if err2 != nil {
		t.Errorf("expected second request to succeed, got error %v", err2)
	}
}

type syncStore struct {
	underlying        Store
	newRequestCreated chan struct{}
	oldReqID          string
	newReqID          string
	mu                sync.Mutex
}

func (s *syncStore) Create(ctx context.Context, req *Request) error {
	s.mu.Lock()
	if s.oldReqID == "" {
		s.oldReqID = req.ID
	} else if req.ID != s.oldReqID {
		s.newReqID = req.ID
	}
	s.mu.Unlock()

	err := s.underlying.Create(ctx, req)

	s.mu.Lock()
	if s.newReqID != "" && req.ID == s.newReqID {
		select {
		case <-s.newRequestCreated:
		default:
			close(s.newRequestCreated)
		}
	}
	s.mu.Unlock()

	return err
}

func (s *syncStore) Get(ctx context.Context, id string) (*Request, error) {
	return s.underlying.Get(ctx, id)
}

func (s *syncStore) GetBySessionID(ctx context.Context, sessionID string) (*Request, error) {
	return s.underlying.GetBySessionID(ctx, sessionID)
}

func (s *syncStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	oldID := s.oldReqID
	s.mu.Unlock()

	if id == oldID {
		<-s.newRequestCreated
	}
	return s.underlying.Delete(ctx, id)
}

func TestSafeDeletionConcurrency(t *testing.T) {
	memStore := NewMemoryStore()
	notifier := &mockNotifier{}
	logger := &mockLogger{}

	syncStoreWrapper := &syncStore{
		underlying:        memStore,
		newRequestCreated: make(chan struct{}),
	}

	manager := NewManager(syncStoreWrapper, notifier, 1*time.Second, logger)

	questions := []Question{
		{Question: "Choose one", Type: SingleSelect, Options: []string{"A", "B"}},
	}

	sessionID := "session-5"

	req1Created := make(chan string, 1)
	notifier.notifyFunc = func(ctx context.Context, req *Request) error {
		req1Created <- req.ID
		return nil
	}

	errCh1 := make(chan error, 1)
	go func() {
		_, err := manager.Elicit(context.Background(), sessionID, questions)
		errCh1 <- err
	}()

	var oldID string
	select {
	case oldID = <-req1Created:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for first request creation")
	}

	req2Created := make(chan string, 1)
	notifier.notifyFunc = func(ctx context.Context, req *Request) error {
		req2Created <- req.ID
		return nil
	}

	errCh2 := make(chan error, 1)
	go func() {
		_, err := manager.Elicit(context.Background(), sessionID, questions)
		errCh2 <- err
	}()

	var newID string
	select {
	case newID = <-req2Created:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for second request creation")
	}

	// Wait for Elicit-1 to return and complete its defer (which cleans up req1)
	err1 := <-errCh1
	if !errors.Is(err1, ErrSessionCancelled) {
		t.Errorf("expected ErrSessionCancelled error, got %v", err1)
	}

	// Check the store now before we resolve Elicit-2. At this point, Elicit-1's defer
	// has run, but Elicit-2's defer has NOT run because it is still active.
	currentReq, err := memStore.GetBySessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("new request lost from bySession mapping! error: %v", err)
	}
	if currentReq.ID != newID {
		t.Errorf("expected request in session to be %s, got %s", newID, currentReq.ID)
	}

	// Now resolve Elicit-2 to let it finish
	ans := Result{Answers: []Answer{{Question: "Choose one", Selected: []string{"B"}}}}
	if err := manager.Resolve(context.Background(), newID, ans); err != nil {
		t.Fatalf("failed to resolve second request: %v", err)
	}

	err2 := <-errCh2
	if err2 != nil {
		t.Errorf("expected second request to succeed, got error %v", err2)
	}

	// Double check old request is indeed deleted from byID
	_, err = memStore.Get(context.Background(), oldID)
	if !errors.Is(err, ErrRequestNotFound) {
		t.Errorf("expected old request to be deleted, but found in byID")
	}
}

func TestHTTPResolveHandler(t *testing.T) {
	store := NewMemoryStore()
	manager := NewManager(store, &mockNotifier{}, 1*time.Second, nil)
	handler := NewHTTPHandler(manager)

	// Setup a pending request channel manually
	resCh := make(chan resultOrErr, 1)
	manager.mu.Lock()
	manager.pending["test-req-id"] = resCh
	manager.mu.Unlock()

	// 1. Success case
	body := `{"id": "test-req-id", "answers": [{"question": "Q1", "selected": ["A"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/resolve", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ResolveHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok: true, got %v", resp)
	}

	// Check channel received the correct result
	select {
	case res := <-resCh:
		if len(res.result.Answers) != 1 || res.result.Answers[0].Question != "Q1" {
			t.Errorf("incorrect result in channel: %+v", res.result)
		}
	default:
		t.Error("expected channel to receive result")
	}

	// 2. Method not allowed case
	req = httptest.NewRequest(http.MethodGet, "/resolve", nil)
	w = httptest.NewRecorder()
	handler.ResolveHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}

	// 3. Malformed JSON case
	req = httptest.NewRequest(http.MethodPost, "/resolve", strings.NewReader("{invalid json"))
	w = httptest.NewRecorder()
	handler.ResolveHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 4. Request not found (or already resolved) case
	bodyNotFound := `{"id": "non-existent-id", "answers": []}`
	req = httptest.NewRequest(http.MethodPost, "/resolve", strings.NewReader(bodyNotFound))
	w = httptest.NewRecorder()
	handler.ResolveHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHTTPGetPendingHandler(t *testing.T) {
	store := NewMemoryStore()
	manager := NewManager(store, &mockNotifier{}, 1*time.Second, nil)
	handler := NewHTTPHandler(manager)

	// Create a request in store
	reqObj := &Request{
		ID:        "req-123",
		SessionID: "session-123",
		Questions: []Question{{Question: "Q1", Type: SingleSelect, Options: []string{"A"}}},
		CreatedAt: time.Now(),
	}
	_ = store.Create(context.Background(), reqObj)

	// 1. Success case
	req := httptest.NewRequest(http.MethodGet, "/pending?session_id=session-123", nil)
	w := httptest.NewRecorder()
	handler.GetPendingHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var retrieved Request
	if err := json.Unmarshal(w.Body.Bytes(), &retrieved); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}
	if retrieved.ID != "req-123" || retrieved.SessionID != "session-123" {
		t.Errorf("incorrect retrieved request: %+v", retrieved)
	}

	// 2. Method not allowed
	req = httptest.NewRequest(http.MethodPost, "/pending?session_id=session-123", nil)
	w = httptest.NewRecorder()
	handler.GetPendingHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}

	// 3. Missing query param
	req = httptest.NewRequest(http.MethodGet, "/pending", nil)
	w = httptest.NewRecorder()
	handler.GetPendingHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 4. Request not found
	req = httptest.NewRequest(http.MethodGet, "/pending?session_id=unknown-session", nil)
	w = httptest.NewRecorder()
	handler.GetPendingHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
