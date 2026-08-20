package media

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

type EventBroadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

var GlobalBroadcaster = &EventBroadcaster{
	clients: make(map[chan []byte]bool),
}

func (b *EventBroadcaster) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 50)
	b.clients[ch] = true
	log.Debug().Msg("New SSE client subscribed")
	return ch
}

func (b *EventBroadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
		log.Debug().Msg("SSE client unsubscribed")
	}
}

func (b *EventBroadcaster) Broadcast(data interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	payload, err := json.Marshal(data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal SSE payload")
		return
	}

	for ch := range b.clients {
		select {
		case ch <- payload:
		default:
			log.Warn().Msg("Dropping SSE message for slow client")
		}
	}
}

func EventsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	ch := GlobalBroadcaster.Subscribe()
	defer GlobalBroadcaster.Unsubscribe(ch)

	// Send initial snapshot of queue
	snapshot := GlobalQueue.GetTasks()
	initialPayload, _ := json.Marshal(map[string]interface{}{
		"type":  "init",
		"tasks": snapshot,
	})
	fmt.Fprintf(w, "data: %s\n\n", initialPayload)
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
