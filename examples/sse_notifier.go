package main

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Hieun003/sonar"
)

// SSEBroadcaster broadcasts elicitation requests to subscribed SSE clients.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[string][]chan string // sessionID -> list of client channels
}

// NewSSEBroadcaster creates a new SSEBroadcaster instance.
func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		clients: make(map[string][]chan string),
	}
}

// Notify marshals the request to JSON and broadcasts it to all subscribed clients of the request's SessionID.
func (b *SSEBroadcaster) Notify(ctx context.Context, req *elicit.Request) error {
	b.mu.RLock()
	chans, exists := b.clients[req.SessionID]
	b.mu.RUnlock()

	if !exists || len(chans) == 0 {
		return nil
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	payloadStr := string(payload)

	for _, ch := range chans {
		select {
		case ch <- payloadStr:
		default:
			// Skip slow client
		}
	}

	return nil
}

// Subscribe registers a new client channel to listen for messages on a sessionID.
func (b *SSEBroadcaster) Subscribe(sessionID string) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan string, 10)
	b.clients[sessionID] = append(b.clients[sessionID], ch)
	return ch
}

// Unsubscribe removes a client channel subscription for a sessionID.
func (b *SSEBroadcaster) Unsubscribe(sessionID string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chans, ok := b.clients[sessionID]
	if !ok {
		return
	}

	for i, c := range chans {
		if c == ch {
			b.clients[sessionID] = append(chans[:i], chans[i+1:]...)
			close(c)
			break
		}
	}

	if len(b.clients[sessionID]) == 0 {
		delete(b.clients, sessionID)
	}
}
