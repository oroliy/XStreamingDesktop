package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsoles(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v6/servers/home" || r.URL.Query().Get("mr") != "50" || r.Header.Get("X-MS-Device-Info") == "" {
			t.Error("invalid request")
		}
		w.Write([]byte(`{"results":[{"serverId":"id","deviceName":"Xbox"}]}`))
	}))
	defer s.Close()
	c, _ := NewClient(s.URL, "token", "home", s.Client())
	consoles, err := c.Consoles(context.Background())
	if err != nil || len(consoles) != 1 || consoles[0].Name != "Xbox" {
		t.Fatalf("%v %v", consoles, err)
	}
}

func TestConsolesMissingResults(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer s.Close()
	c, _ := NewClient(s.URL, "token", "home", s.Client())
	if _, err := c.Consoles(context.Background()); err == nil {
		t.Fatal("accepted missing results")
	}
}
