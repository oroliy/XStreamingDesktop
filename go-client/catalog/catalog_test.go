package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolve(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("market") != "US" || r.Header.Get("calling-app-name") != "Xbox Cloud Gaming Web" {
			t.Error("bad request")
		}
		var body struct{ Products []string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Products) > 100 {
			t.Error("batch too big")
		}
		fmt.Fprint(w, `{"Products":{"ID0":{"ProductTitle":"A Game"}}}`)
	}))
	defer s.Close()
	titles := make([]streaming.Title, 101)
	for i := range titles {
		titles[i].TitleID = fmt.Sprint(i)
		titles[i].Details.ProductID = fmt.Sprintf("id%d", i)
	}
	got, err := (&Client{http: s.Client(), endpoint: s.URL}).Resolve(context.Background(), titles, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(got) != 101 || got[0].Name != "A Game" {
		t.Fatalf("calls=%d titles=%d first=%+v", calls, len(got), got[0])
	}
}
