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

	"github.com/asymmetric/deploy-dashboard/internal/pin"
)

// Client polls a topic URL, e.g. https://ntfy.example.com/pins, that the
// pin effects publish their records to:
//
//	curl -d '{"host":"…","commit":"…","toplevel":"/nix/store/…"}' <ntfy>/pins
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
// ("all" for every cached message), and the id of the last message read, to
// pass as since next time (since itself if there was none). Messages that
// aren't valid records are skipped.
func (c *Client) Poll(ctx context.Context, since string) (recs []pin.Record, cursor string, err error) {
	cursor = since
	if since == "" {
		since = "all"
	}
	u := strings.TrimRight(c.TopicURL, "/") + "/json?poll=1&since=" + url.QueryEscape(since)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, cursor, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, cursor, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, cursor, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil || m.Event != "message" {
			continue
		}
		cursor = m.ID
		var r pin.Record
		if err := json.Unmarshal([]byte(m.Message), &r); err != nil || r.Host == "" || r.Toplevel == "" {
			continue
		}
		r.Time = time.Unix(m.Time, 0)
		recs = append(recs, r)
	}
	return recs, cursor, sc.Err()
}
