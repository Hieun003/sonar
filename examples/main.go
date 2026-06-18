package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Hieun003/sonar"
)

//go:embed index.html
var indexHTML []byte

var (
	store       = elicit.NewMemoryStore()
	broadcaster = NewSSEBroadcaster()
	manager     = elicit.NewManager(store, broadcaster, 2*time.Minute, slog.Default())
	httpHandler = elicit.NewHTTPHandler(manager)
)

var questions = []elicit.Question{
	{Question: "Bạn muốn tiếp tục không?", Type: elicit.SingleSelect, Options: []string{"Có", "Không"}},
	{Question: "Chọn các tính năng cần thiết", Type: elicit.MultiSelect, Options: []string{"Tốc độ", "Độ chính xác", "Chi phí thấp"}},
	{Question: "Sắp xếp ưu tiên", Type: elicit.RankPriority, Options: []string{"Hiệu suất", "Bảo mật", "Dễ dùng"}},
}

func main() {
	// Set Gin to release mode to keep stdout logs clean
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	// Serve client HTML via embedded index.html
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	// SSE stream connection endpoint
	r.GET("/events", handleEvents)

	// Mock agent trigger endpoint
	r.POST("/trigger", handleTrigger)

	// Resolve & GetPending routes using go-elicit's standard HTTPHandler methods
	r.POST("/resolve", gin.WrapF(httpHandler.ResolveHandler))
	r.GET("/pending", gin.WrapF(httpHandler.GetPendingHandler))

	log.Println("==================================================")
	log.Println(" go-elicit Demo Server started at http://localhost:8080")
	log.Println("==================================================")

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Server failed to run: %v", err)
	}
}

func handleEvents(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	ch := broadcaster.Subscribe(sessionID)
	defer broadcaster.Unsubscribe(sessionID, ch)

	// Set headers required for Server-Sent Events
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush() // Flush headers immediately to complete EventSource handshake

	// Stream messages to the client
	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-ch:
			if !ok {
				return false
			}
			_, err := fmt.Fprintf(w, "data: %s\n\n", msg)
			return err == nil
		case <-c.Request.Context().Done():
			return false
		}
	})
}

func handleTrigger(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Trigger the elicitation process in the background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		log.Printf("[Agent] Starting elicitation for session: %s", req.SessionID)
		res, err := manager.Elicit(ctx, req.SessionID, questions)
		if err != nil {
			log.Printf("[Agent] Elicit failed for session %s: %v", req.SessionID, err)
			return
		}
		log.Printf("[Agent] Elicit completed successfully for session %s! Result: %+v", req.SessionID, res)
	}()

	c.Status(http.StatusAccepted)
}
