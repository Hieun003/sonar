# Sonar (`go-elicit`)

[![Go Version](https://img.shields.io/github/go-mod/go-version/Hieun003/sonar?filename=go-elicit%2Fgo.mod)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Hieun003/sonar)](https://goreportcard.com/report/github.com/Hieun003/sonar)

Sonar is a concurrency-safe, developer-focused Go library designed to facilitate synchronous **Human-in-the-Loop (HITL)** elicitation workflows for LLM (Large Language Model) Agents.

## Overview

When building advanced AI Agents, there are scenarios where the LLM does not have enough information to proceed and must ask a human operator for input before continuing. 

Sonar solves this by providing a clean, thread-safe orchestration layer. It allows your Agent's background goroutine to issue an elicitation request and block synchronously. Sonar notifies the frontend client, caches the pending request, waits for the operator's submission via HTTP, and then wakes up the agent's goroutine with the human's answers—complete with built-in support for timeouts, context cancellation, and multi-session safety.

---

## Features

- **Synchronous Elicitation**: Block goroutines synchronously until responses are resolved.
- **Robust Concurrency Control**: Lock-free session mapping to prevent race conditions during overlaps.
- **Timeout & Cancellation Handling**: Leverages native Go `time.Timer` and `context.Context` cancellation.
- **Pluggable Storage**: Built-in MemoryStore, easily extendable to Redis, PostgreSQL, or NATS KV.
- **Pluggable Notification**: Define how to alert clients (e.g. SSE, WebSocket, NATS, Webhooks).
- **Stdlib HTTP Handlers**: Standard `net/http` handlers provided out of the box.
- **LLM Adapters**: Built-in Tool Calling definitions and argument parsers for OpenAI and Anthropic Claude.
- **Zero Dependencies**: Core codebase depends only on the standard library and `uuid`.

---

## Installation

```bash
go get github.com/Hieun003/sonar
```

---

## Quick Start

The following example shows how to set up the Manager, trigger elicitation in one goroutine, and resolve it from another:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Hieun003/sonar"
)

type consoleNotifier struct{}

func (n *consoleNotifier) Notify(ctx context.Context, req *elicit.Request) error {
	fmt.Printf("[Notify] Elicitation required for session %s (Request ID: %s)\n", req.SessionID, req.ID)
	return nil
}

func main() {
	// 1. Initialize store, notifier, and manager
	store := elicit.NewMemoryStore()
	notifier := &consoleNotifier{}
	manager := elicit.NewManager(store, notifier, 10*time.Second, nil)

	ctx := context.Background()
	sessionID := "session-123"

	// 2. Simulate human answering the questionnaire in the background
	go func() {
		time.Sleep(1 * time.Second) // wait for notification to print

		// Look up pending request
		req, err := store.GetBySessionID(ctx, sessionID)
		if err != nil {
			log.Printf("Store lookup failed: %v", err)
			return
		}

		// Resolve it
		ans := elicit.Result{
			Answers: []elicit.Answer{
				{Question: "Do you want to proceed?", Selected: []string{"Yes"}},
			},
		}
		if err := manager.Resolve(ctx, req.ID, ans); err != nil {
			log.Printf("Failed to resolve request: %v", err)
		}
	}()

	// 3. Elicit questions (blocks the current goroutine)
	questions := []elicit.Question{
		{
			Question: "Do you want to proceed?",
			Type:     elicit.SingleSelect,
			Options:  []string{"Yes", "No"},
		},
	}

	fmt.Println("[Agent] Initiating elicitation...")
	result, err := manager.Elicit(ctx, sessionID, questions)
	if err != nil {
		log.Fatalf("Elicitation failed: %v", err)
	}

	fmt.Printf("[Agent] Resumed! Result received: %+v\n", result.Answers)
}
```

---

## HTTP Integration

You can easily mount Sonar's standard HTTP endpoints directly onto any `net/http` router (like `http.ServeMux` or inside custom middlewares):

```go
handler := elicit.NewHTTPHandler(manager)

mux := http.NewServeMux()
// POST /resolve (Accepts {"id": "uuid", "answers": [...]})
mux.HandleFunc("/resolve", handler.ResolveHandler)

// GET /pending?session_id=... (Retrieves pending Request details)
mux.HandleFunc("/pending", handler.GetPendingHandler)
```

---

## LLM Adapter Usage

Adapters isolate model-specific schemas, ensuring your core application logic is not coupled to a specific vendor.

### OpenAI (Tool Calling)

```go
import "github.com/Hieun003/sonar/adapters/openai"

// 1. Retrieve the tool definition schema to pass to the OpenAI chat completion API
toolDef := openaiadapter.GetToolDefinition()

// 2. Parse raw tool call arguments JSON string returned by GPT models
questions, err := openaiadapter.ParseToolArguments(toolCall.Arguments)
if err != nil {
    log.Fatalf("Invalid arguments: %v", err)
}

// 3. Block agent wait for human response
result, err := manager.Elicit(ctx, sessionID, questions)
```

### Anthropic Claude (Tool Use)

```go
import "github.com/Hieun003/sonar/adapters/anthropic"

// 1. Retrieve the tool definition schema to pass to the Anthropic messages API
toolDef := anthropicadapter.GetToolDefinition()

// 2. Parse raw tool use input JSON string returned by Claude models
questions, err := anthropicadapter.ParseToolInput(toolUse.InputJSON)
if err != nil {
    log.Fatalf("Invalid input: %v", err)
}

// 3. Block agent wait for human response
result, err := manager.Elicit(ctx, sessionID, questions)
```

---

## Implementing a Custom Notifier

To alert client applications when questions are pending, implement the `elicit.Notifier` interface:

```go
type MyNotifier struct {
    // fields for SSE, WebSocket client hub, NATS broker, etc.
}

func (n *MyNotifier) Notify(ctx context.Context, req *elicit.Request) error {
    // Marshal payload to JSON and push to client channels
    payload, err := json.Marshal(req)
    if err != nil {
        return err
    }
    return n.broadcast(req.SessionID, payload)
}
```

---

## Architecture

```
LLM Agent Goroutine          Backend Service           Browser / UI Client
───────────────────          ───────────────           ───────────────────
manager.Elicit(...)  ─────►   Create request    
        │                     Save to Store
        │ (blocks)            Notifier.Notify() ─────► (SSE / WebSocket Alert)
        │                                              Renders HTML Form
        │                                              User submits choices
        │                     Resolve(reqID)    ◄───── POST /resolve
        ▼                            │
Result received ◄────────────────────┘
```

---

## Running the Demo

The repository contains a fully working demonstration server utilizing SSE and Gin.

```bash
# Clone the repository
git clone https://github.com/Hieun003/sonar
cd sonar/go-elicit

# Run the example server
go run ./examples/

# Open your browser and navigate to:
# http://localhost:8080
```

---

## Sentinel Errors

The following standard error variables are exported for error checking with `errors.Is`:

| Error | Package | Description |
| :--- | :--- | :--- |
| `ErrTimeout` | `elicit` | Returned when the elicitation request takes longer than the timeout duration. |
| `ErrSessionCancelled` | `elicit` | Sent to pending requests if a new request is started on the same SessionID. |
| `ErrNotFoundOrResolved` | `elicit` | Returned by `Resolve` if the request is missing or already completed. |
| `ErrParseToolArguments` | `openaiadapter` | Returned when OpenAI tool arguments fail to decode. |
| `ErrParseToolInput` | `anthropicadapter` | Returned when Anthropic tool input fails to decode. |
| `ErrRequestNotFound` | `elicit` | Returned by `MemoryStore` when querying a non-existent request ID. |

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
