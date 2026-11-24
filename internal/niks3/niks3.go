// Package niks3 reads host pins from niks3's read proxy.
package niks3

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client reads pins from a niks3 server, e.g. http://niks3.example.ts.net:5751.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// Pin returns the store path pinned for host (GET <base>/pins/<host>).
func (c *Client) Pin(ctx context.Context, host string) (string, error) {
	url := strings.TrimRight(c.BaseURL, "/") + "/pins/" + host
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	path := strings.TrimSpace(string(body))
	if !strings.HasPrefix(path, "/nix/store/") {
		return "", fmt.Errorf("GET %s: unexpected body %q", url, path)
	}
	return path, nil
}
