package streaming

import (
	"context"
	"errors"
	"net/http"
)

type Title struct {
	TitleID string `json:"titleId"`
	Details struct {
		ProductID string `json:"productId"`
	} `json:"details"`
	Name string `json:"-"`
}

// Titles lists titles available to the cloud account. Name is populated separately by catalog lookup.
func (c *Client) Titles(ctx context.Context) ([]Title, error) {
	if c.kind != "cloud" {
		return nil, errors.New("titles require cloud streaming")
	}
	var response struct {
		Results []Title `json:"results"`
	}
	_, err := c.request(ctx, http.MethodGet, "/v2/titles", nil, &response)
	return response.Results, err
}
