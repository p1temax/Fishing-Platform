package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type qrRelayEvent struct {
	Type           string `json:"type"`
	UploadCount    int64  `json:"upload_count,omitempty"`
	At             string `json:"at,omitempty"`
	PayloadHash    string `json:"payload_hash,omitempty"`
	PayloadChanged bool   `json:"payload_changed,omitempty"`
	FrameID        uint   `json:"frame_id,omitempty"`
}

type qrRelayHub struct {
	mu   sync.RWMutex
	subs map[uint]map[chan qrRelayEvent]struct{}
}

var qrEvents = &qrRelayHub{subs: map[uint]map[chan qrRelayEvent]struct{}{}}

func (h *qrRelayHub) subscribe(relayID uint) chan qrRelayEvent {
	ch := make(chan qrRelayEvent, 8)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[relayID] == nil {
		h.subs[relayID] = map[chan qrRelayEvent]struct{}{}
	}
	h.subs[relayID][ch] = struct{}{}
	return ch
}

func (h *qrRelayHub) unsubscribe(relayID uint, ch chan qrRelayEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[relayID]; ok {
		delete(set, ch)
		if len(set) == 0 {
			delete(h.subs, relayID)
		}
	}
	close(ch)
}

func (h *qrRelayHub) publish(relayID uint, ev qrRelayEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[relayID] {
		select {
		case ch <- ev:
		default:
			// drop if subscriber is slow
		}
	}
}

// StreamQrRelayEvents streams SSE frame notifications for console preview.
func StreamQrRelayEvents(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	ch := qrEvents.subscribe(row.ID)
	defer qrEvents.unsubscribe(row.ID, ch)

	// initial ping
	fmt.Fprintf(c.Writer, "event: ping\ndata: {}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			fmt.Fprintf(c.Writer, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			raw, _ := json.Marshal(ev)
			fmt.Fprintf(c.Writer, "event: frame\ndata: %s\n\n", raw)
			flusher.Flush()
		}
	}
}

var (
	qrUploadRateMu sync.Mutex
	qrUploadHits   = map[uint][]time.Time{}
)

const qrUploadRateLimitPerSec = 5

func allowQrUpload(relayID uint) bool {
	now := time.Now()
	qrUploadRateMu.Lock()
	defer qrUploadRateMu.Unlock()
	windowStart := now.Add(-1 * time.Second)
	hits := qrUploadHits[relayID]
	kept := hits[:0]
	for _, t := range hits {
		if t.After(windowStart) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= qrUploadRateLimitPerSec {
		qrUploadHits[relayID] = kept
		return false
	}
	kept = append(kept, now)
	qrUploadHits[relayID] = kept
	return true
}
