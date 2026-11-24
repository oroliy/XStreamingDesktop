package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeviceAndRefresh(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("client_id") != ClientID {
			t.Error("missing client ID")
		}
		switch r.URL.Path {
		case "/devicecode":
			w.Write([]byte(`{"device_code":"dev","user_code":"abc","expires_in":60,"interval":1}`))
		case "/token":
			if r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") != "refresh" {
				t.Error("missing refresh token")
			}
			w.Write([]byte(`{"access_token":"secret","refresh_token":"refresh"}`))
		default:
			t.Error("unexpected path")
		}
	}))
	defer s.Close()
	c := &Client{endpoint: s.URL, http: s.Client()}
	code, err := c.Start(context.Background())
	if err != nil || code.UserCode != "abc" {
		t.Fatalf("%+v %v", code, err)
	}
	token, err := c.Poll(context.Background(), code)
	if err != nil || token.AccessToken != "secret" {
		t.Fatalf("token failed: %v", err)
	}
	_, err = c.Refresh(context.Background(), "refresh")
	if err != nil {
		t.Fatal(err)
	}
}
func TestPollCancellation(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := c.Poll(ctx, DeviceCode{DeviceCode: "x", ExpiresIn: 10, Interval: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
