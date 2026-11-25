// Package nixbot reads builds and effect runs from nixbot's JSON API
// (https://github.com/Mic92/nixbot, /api, OpenAPI at /openapi.json).
package nixbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Build statuses.
const (
	BuildPending    = "pending"
	BuildEvaluating = "evaluating"
	BuildBuilding   = "building"
	BuildSucceeded  = "succeeded"
	BuildFailed     = "failed"
	BuildCancelled  = "cancelled"
)

// Effect statuses. An effect skipped by when.changed is succeeded, see
// Effect.Unchanged.
const (
	EffectPending   = "pending"
	EffectRunning   = "running"
	EffectSucceeded = "succeeded"
	EffectFailed    = "failed"
	EffectSkipped   = "skipped"
	EffectCancelled = "cancelled"
)

// KindPush is the kind of the effect runs of a default-branch build.
const KindPush = "push"

// Build is one CI run of a commit.
type Build struct {
	Number    int     `json:"number"`
	CommitSHA string  `json:"commit_sha"`
	Branch    string  `json:"branch"`
	PRNumber  *int    `json:"pr_number"`
	Status    string  `json:"status"`
	Error     *string `json:"error"`
}

// Effect is one effect run of a build.
type Effect struct {
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Error  *string `json:"error"`
	// ReusedFrom is the id of the earlier run whose when.changed inputs
	// matched, when nixbot settled this one as succeeded without running it.
	ReusedFrom *int64 `json:"reused_from"`
}

// Unchanged reports whether the effect succeeded without running, because
// its when.changed inputs are those of an earlier successful run.
func (e Effect) Unchanged() bool {
	return e.ReusedFrom != nil
}

// HasLog reports whether the effect has a log: it ran, or is running.
func (e Effect) HasLog() bool {
	switch e.Status {
	case EffectPending, EffectSkipped:
		return false
	case EffectSucceeded:
		return !e.Unchanged()
	}
	return true
}

// Running reports whether b hasn't finished yet.
func (b Build) Running() bool {
	switch b.Status {
	case BuildPending, BuildEvaluating, BuildBuilding:
		return true
	}
	return false
}

// Client reads one repository's builds from a nixbot server.
type Client struct {
	// BaseURL is nixbot's web URL, e.g. https://nixbot.example.ts.net.
	BaseURL string
	// Repo is the repository as it appears in nixbot's URLs:
	// <forge>/<owner>/<name>, e.g. github/owner/dotfiles.
	Repo string
	// Token is a personal API token (nixbot's /settings); empty for none,
	// which only works for public repositories.
	Token string
	HTTP  *http.Client
}

// BuildURL is the web page of build n.
func (c *Client) BuildURL(n int) string {
	return c.repoURL() + "/builds/" + strconv.Itoa(n)
}

// EffectLogURL is the log of effect name (kind push) of build n.
func (c *Client) EffectLogURL(n int, name string) string {
	return c.BuildURL(n) + "/logs/effect:" + url.PathEscape(name)
}

func (c *Client) repoURL() string {
	return strings.TrimRight(c.BaseURL, "/") + "/repos/" + c.Repo
}

// Latest returns the newest build of branch that wasn't cancelled (a newer
// push cancels older builds), with its effects. It looks at the first page
// of builds only; ok is false if there is no such build there.
func (c *Client) Latest(ctx context.Context, branch string) (b Build, effects []Effect, ok bool, err error) {
	var page struct {
		Items []Build `json:"items"`
	}
	if err := c.get(ctx, "/builds?"+url.Values{"branch": {branch}}.Encode(), &page); err != nil {
		return Build{}, nil, false, err
	}
	for _, it := range page.Items {
		// a PR's head branch may be named like the default branch
		if it.PRNumber == nil && it.Status != BuildCancelled {
			b, ok = it, true
			break
		}
	}
	if !ok {
		return Build{}, nil, false, nil
	}
	var detail struct {
		Effects []Effect `json:"effects"`
	}
	if err := c.get(ctx, "/builds/"+strconv.Itoa(b.Number), &detail); err != nil {
		return Build{}, nil, false, err
	}
	return b, detail.Effects, true, nil
}

// get decodes GET <base>/api/repos/<repo><path> into v.
func (c *Client) get(ctx context.Context, path string, v any) error {
	u := strings.TrimRight(c.BaseURL, "/") + "/api/repos/" + c.Repo + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 404 also means "not visible": a missing or wrong token
		return fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v); err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	return nil
}
