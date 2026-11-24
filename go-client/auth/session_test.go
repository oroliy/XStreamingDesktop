package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionRotatesRefreshToken(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			_ = r.ParseForm()
			if r.Form.Get("refresh_token") != "initial" {
				t.Error("wrong initial refresh token")
			}
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated"}`)
		case "/user":
			fmt.Fprint(w, `{"Token":"user"}`)
		case "/xsts":
			fmt.Fprint(w, `{"Token":"xsts"}`)
		case "/stream":
			fmt.Fprint(w, `{"gsToken":"gs","offeringSettings":{"regions":[{"baseUri":"https://example.com","isDefault":true}]}}`)
		case "/transfer":
			_ = r.ParseForm()
			if r.Form.Get("refresh_token") != "rotated" {
				t.Error("refresh token not rotated")
			}
			fmt.Fprint(w, `{"access_token":"transfer"}`)
		default:
			t.Error("unexpected path")
		}
	}))
	defer s.Close()
	session := &Session{refresh: "initial", microsoft: &Client{endpoint: s.URL + "/oauth", transferURL: s.URL + "/transfer", http: s.Client()}, xbox: &XboxClient{http: s.Client(), userURL: s.URL + "/user", xstsURL: s.URL + "/xsts", homeURL: s.URL + "/stream"}}
	token, region, err := session.StreamingToken(context.Background(), "home")
	if err != nil || token != "gs" || region != "https://example.com" {
		t.Fatal(err)
	}
	transfer, err := session.Transfer(context.Background())
	if err != nil || transfer != "transfer" {
		t.Fatal(err)
	}
}
