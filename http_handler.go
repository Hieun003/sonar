package elicit

import (
	"encoding/json"
	"errors"
	"net/http"
)

// HTTPHandler provides net/http handlers for resolving and retrieving elicitation requests.
type HTTPHandler struct {
	manager *Manager
}

// NewHTTPHandler creates and returns a new HTTPHandler.
func NewHTTPHandler(manager *Manager) *HTTPHandler {
	return &HTTPHandler{
		manager: manager,
	}
}

// resolveRequest defines the payload structure for resolving a request.
type resolveRequest struct {
	ID      string   `json:"id"`
	Answers []Answer `json:"answers"`
}

// ResolveHandler handles HTTP POST requests to resolve a pending elicitation request.
// It returns:
// - 405 Method Not Allowed if the request method is not POST.
// - 400 Bad Request if the request body is malformed or invalid JSON.
// - 404 Not Found if the request ID is not found or has already been resolved.
// - 500 Internal Server Error for any other error.
// - 200 OK with {"ok": true} on success.
func (h *HTTPHandler) ResolveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
		return
	}

	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid request body"}`))
		return
	}

	err := h.manager.Resolve(r.Context(), req.ID, Result{Answers: req.Answers})
	if err != nil {
		if errors.Is(err, ErrNotFoundOrResolved) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "request not found or already resolved"}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal error"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok": true}`))
}

// GetPendingHandler handles HTTP GET requests to retrieve the pending request of a session.
// It returns:
// - 405 Method Not Allowed if the request method is not GET.
// - 400 Bad Request if the session_id query parameter is missing or empty.
// - 404 Not Found if no pending request exists for the session.
// - 500 Internal Server Error for any other error.
// - 200 OK with the JSON payload of the Request struct on success.
func (h *HTTPHandler) GetPendingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "session_id is required"}`))
		return
	}

	req, err := h.manager.GetPending(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, ErrRequestNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "no pending request for session"}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal error"}`))
		return
	}

	if req == nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "no pending request for session"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(req)
}
