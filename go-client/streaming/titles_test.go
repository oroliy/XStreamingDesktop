package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudTitles(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/titles" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"results":[{"titleId":"game","details":{"productId":"product"}}]}`))
	}))
	defer s.Close()
	c, _ := NewClient(s.URL, "token", "cloud", s.Client())
	titles, err := c.Titles(context.Background())
	if err != nil || len(titles) != 1 || titles[0].TitleID != "game" || titles[0].Details.ProductID != "product" {
		t.Fatal(titles, err)
	}
}

func TestMissingResultsIsError(t *testing.T) {
	for _, body := range []string{`{}`, `{"results":null}`, ``} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		c, _ := NewClient(s.URL, "token", "cloud", s.Client())
		_, err := c.Titles(context.Background())
		s.Close()
		if err == nil {
			t.Fatalf("accepted missing results in %q", body)
		}
	}
}
func TestEmptyResultsIsValid(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"results":[]}`)) }))
	defer s.Close()
	c, _ := NewClient(s.URL, "token", "cloud", s.Client())
	titles, err := c.Titles(context.Background())
	if err != nil || len(titles) != 0 {
		t.Fatal(titles, err)
	}
}
