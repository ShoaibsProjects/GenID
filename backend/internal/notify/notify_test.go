package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNotifySlack(t *testing.T) {
	var mu sync.Mutex
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewFromURLs(srv.URL, "")
	err := n.Notify(context.Background(), Event{
		Event:        "approval.required",
		RequestID:    "abcd1234-0000-0000-0000-000000000000",
		RequestType:  "access.request.grant",
		Level:        1,
		ApproverRole: "resource_owner",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatal("no payload received")
	}
	text, _ := got["text"].(string)
	if text == "" {
		t.Fatalf("slack payload missing text: %v", got)
	}
}

func TestNotifyTeams(t *testing.T) {
	var mu sync.Mutex
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewFromURLs("", srv.URL)
	if err := n.Notify(context.Background(), Event{
		Event:      "approval.decided",
		RequestID:  "abcd1234-0000-0000-0000-000000000000",
		Level:      2,
		Status:     "denied",
		ApproverID: "00000000-0000-0000-0000-000000000c0c",
		Comment:    "Not justified",
	}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got["@type"] != "MessageCard" {
		t.Fatalf("teams payload missing MessageCard type: %v", got)
	}
}

func TestNotifyNoWebhook(t *testing.T) {
	n := NewFromURLs("", "")
	if err := n.Notify(context.Background(), Event{Event: "approval.required", RequestID: "x"}); err != nil {
		t.Fatalf("no-webhook notify should succeed silently: %v", err)
	}
}

func TestNotifyWebhookError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewFromURLs(srv.URL, "")
	err := n.Notify(context.Background(), Event{Event: "approval.required", RequestID: "x"})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}