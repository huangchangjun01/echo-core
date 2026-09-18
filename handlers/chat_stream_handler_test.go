package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteSSEHeartbeat_Format(t *testing.T) {
	var w http.ResponseWriter = httptest.NewRecorder()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("response writer must support Flusher")
	}
	ts := int64(1737038400123)
	if err := writeSSEHeartbeat(w, flusher, ts); err != nil {
		t.Fatalf("writeSSEHeartbeat: %v", err)
	}
	body := w.(*httptest.ResponseRecorder).Body.String()
	if !strings.HasPrefix(body, ": ping ") {
		t.Errorf("heartbeat must start with ': ping ', got %q", body)
	}
	if !strings.Contains(body, "1737038400123") {
		t.Errorf("heartbeat must contain ts_ms, got %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("heartbeat must end with \\n\\n, got %q", body)
	}
}

func TestWriteSSEHeartbeat_Flushes(t *testing.T) {
	rec := httptest.NewRecorder()
	var w http.ResponseWriter = rec
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("response writer must support Flusher")
	}
	// httptest.ResponseRecorder satisfies Flusher; calling Flush is a no-op but should not panic.
	if err := writeSSEHeartbeat(w, flusher, time.Now().UnixMilli()); err != nil {
		t.Fatalf("writeSSEHeartbeat: %v", err)
	}
	// Verify body was written (httptest buffer).
	if rec.Body.Len() == 0 {
		t.Error("expected body to be written after heartbeat")
	}
}