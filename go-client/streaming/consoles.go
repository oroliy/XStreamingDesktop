package streaming

import (
	"context"
	"errors"
	"net/http"
)

type Console struct {
	ServerID    string `json:"serverId"`
	Name        string `json:"deviceName"`
	ConsoleType string `json:"consoleType"`
	PowerState  string `json:"powerState"`
}

// Consoles lists remote-play consoles available to the signed-in account.
func (c *Client) Consoles(ctx context.Context) ([]Console, error) {
	if c.kind != "home" {
		return nil, errors.New("console list requires home streaming")
	}
	var response struct {
		Results []Console `json:"results"`
	}
	headers := http.Header{}
	headers.Set("X-MS-Device-Info", deviceInfo("windows"))
	_, err := c.requestHeaders(ctx, http.MethodGet, "/v6/servers/home?mr=50", nil, &response, headers)
	if err != nil {
		return nil, err
	}
	if response.Results == nil {
		return nil, errors.New("streaming service returned no results list")
	}
	return response.Results, nil
}
