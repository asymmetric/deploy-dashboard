// Package ntfy polls the ntfy topic that the pin effects publish to.
package ntfy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PinRecord is what a pin-<host> effect publishes after `niks3 push --pin`:
//
//	curl -d '{"host":"…","commit":"…","toplevel":"/nix/store/…"}' <ntfy>/pins
type PinRecord struct {
	Host     string    `json:"host"`
	Commit   string    `json:"commit"`
	Toplevel string    `json:"toplevel"`
	Time     time.Time `json:"time"`
	// MsgID is the ntfy message id, used as the `since` cursor.
	MsgID string `json:"msg_id"`
}

// Client polls a topic URL, e.g. https://ntfy.example.com/pins.
type Client struct {
	TopicURL string
	HTTP     *http.Client
}

type message struct {
	ID      string `json:"id"`
	Time    int64  `json:"time"`
	Event   string `json:"event"`
	Message string `json:"message"`
}

// Poll returns the records published after the message with id since
// ("all" for every cached message). Messages that aren't valid records are skipped.
func (c *Client) Poll(ctx context.Context, since string) ([]PinRecord, error) {
	if since == "" {
		since = "all"
	}
	u := strings.TrimRight(c.TopicURL, "/") + "/json?poll=1&since=" + url.QueryEscape(since)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	var out []PinRecord
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil || m.Event != "message" {
			continue
		}
		var r PinRecord
		if err := json.Unmarshal([]byte(m.Message), &r); err != nil || r.Host == "" || r.Toplevel == "" {
			continue
		}
		r.Time = time.Unix(m.Time, 0)
		r.MsgID = m.ID
		out = append(out, r)
	}
	return out, sc.Err()
}
