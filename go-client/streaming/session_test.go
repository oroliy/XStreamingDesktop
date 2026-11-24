package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestWaitProvisionedConnect(t *testing.T) {
	states := 0
	connected := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v5/sessions/home/abc/connect" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["userToken"] != "transfer" {
				t.Error("wrong token")
			}
			connected = true
			w.WriteHeader(204)
			return
		}
		states++
		if states == 1 {
			fmt.Fprint(w, `{"state":"ReadyToConnect"}`)
		} else {
			fmt.Fprint(w, `{"state":"Provisioned"}`)
		}
	}))
	defer s.Close()
	c, _ := NewClient(s.URL, "token", "home", s.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var progress []string
	if err := c.WaitProvisionedWithProgress(ctx, "abc", func(context.Context) (string, error) { return "transfer", nil }, func(state string) { progress = append(progress, state) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(progress, []string{"ReadyToConnect", "Provisioned"}) {
		t.Fatal(progress)
	}
	if !connected {
		t.Fatal("missing connect")
	}
	if id, err := c.SessionID(PlayResult{SessionPath: "/v5/sessions/abc"}); err != nil || id != "abc" {
		t.Fatal(id, err)
	}
}
