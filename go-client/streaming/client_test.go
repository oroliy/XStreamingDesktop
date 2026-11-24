package streaming

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionRequests(t *testing.T) {
	var calls []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"state":"started"}`))
		case http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer s.Close()
	c, err := NewClient(s.URL, "test-token", "home", s.Client())
	if err != nil {
		t.Fatal(err)
	}
	state, err := c.State(context.Background(), "abc")
	if err != nil || string(state) != `{"state":"started"}` {
		t.Fatalf("state %s: %v", state, err)
	}
	if err := c.Keepalive(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /v5/sessions/home/abc/state", "POST /v5/sessions/home/abc/keepalive", "DELETE /v5/sessions/home/abc"}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls: %v", calls)
		}
	}
}
func TestStateCancellationAndErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer s.Close()
	c, _ := NewClient(s.URL, "token", "cloud", s.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.State(ctx, "abc")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if _, err := c.State(context.Background(), "../bad"); err == nil {
		t.Fatal("accepted invalid ID")
	}
	if _, err := NewClient(s.URL, "token", "other", nil); err == nil {
		t.Fatal("accepted invalid type")
	}
}
