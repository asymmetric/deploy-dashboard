// Package fake serves made-up niks3, comin, nixbot and ntfy backends from
// one HTTP handler, for the end-to-end test and `nix run .#demo`.
//
// Requests are routed by path: /pins/<host> is niks3, /api/... and
// /repos/... are nixbot, /ntfy/pins is the ntfy topic. Requests sent through
// the handler as an HTTP proxy (absolute URLs, e.g. via HTTP_PROXY) are comin
// exporters, keyed by the host in the URL, so hosts keep their real-looking
// names. Go doesn't proxy loopback addresses, so the other backends must be
// addressed as 127.0.0.1.
package fake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/state"
)

// Store paths used by the default scenario.
const (
	PathNew = "/nix/store/2b7q0x9n3f5k1m8c4v6d0s2a1r9p7l3h-nixos-system-host-25.11.20261120"
	PathOld = "/nix/store/9c1f8e2d7a6b5c4d3e2f1a0b9c8d7e6f-nixos-system-host-25.11.20261118"
)

// Host is one host's state across the backends.
type Host struct {
	Name string
	// Pin is the niks3 pin; empty for a 404.
	Pin string

	// Deploy is comin's deployment status (init, running, done, failed);
	// empty if the exporter is unreachable.
	Deploy string
	// Running is comin's out_path; empty to omit the label (upstream comin).
	Running    string
	LastFailed bool

	// RecordToplevel is the toplevel of the pin record CI published, with
	// RecordCommit; empty for no record.
	RecordToplevel string
	RecordCommit   string

	// Effect is the status of the host's pin effect in the newest build;
	// empty if the build has none.
	Effect    string
	EffectErr string

	// Want is the state the dashboard should derive.
	Want state.Kind
}

// Scenario is a set of hosts and the newest default-branch build.
type Scenario struct {
	// Repo is the repository in nixbot's URLs, e.g. github/owner/dotfiles.
	Repo string
	// Token, if set, is the bearer token nixbot requires.
	Token string

	BuildNumber int
	BuildCommit string
	BuildStatus string
	// Effect names are EffectPrefix + host.
	EffectPrefix string

	Hosts []Host
}

// Default covers the states the dashboard derives from reachable backends.
// Offline states need an earlier successful scrape, so they're not in it.
var Default = Scenario{
	Repo:         "github/owner/dotfiles",
	Token:        "secret",
	BuildNumber:  128,
	BuildCommit:  "3f9c2e1d7b6a5c4e8f0a1b2c3d4e5f60718293a4",
	BuildStatus:  "building",
	EffectPrefix: "default.pin-",
	Hosts: []Host{
		{Name: "ogaki", Pin: PathNew, Deploy: "done", Running: PathNew, RecordToplevel: PathNew, Effect: "succeeded", Want: state.InSync},
		{Name: "qirex", Pin: PathNew, Deploy: "done", Running: PathNew, RecordToplevel: PathNew, Effect: "failed",
			EffectErr: "error: builder for '/nix/store/…-etc.drv' failed with exit code 1", Want: state.BuildFailed},
		{Name: "alpha", Pin: PathNew, Deploy: "done", Running: PathNew, RecordToplevel: PathNew, Effect: "running", Want: state.Building},
		{Name: "bokkusu", Pin: PathNew, Deploy: "done", Running: PathOld, RecordToplevel: PathNew, Effect: "pending", Want: state.PendingDeploy},
		{Name: "kappa", Pin: PathNew, Deploy: "running", Running: PathNew, RecordToplevel: PathNew, Effect: "succeeded", Want: state.Deploying},
		{Name: "lambda", Pin: PathNew, Deploy: "failed", Running: PathNew, LastFailed: true, RecordToplevel: PathNew, Effect: "succeeded", Want: state.DeployFailing},
		{Name: "mu", Pin: PathOld, Deploy: "done", Running: PathOld, RecordToplevel: PathNew, Effect: "succeeded", Want: state.PinnedByHand},
		{Name: "nu", Pin: PathNew, Deploy: "done", RecordToplevel: PathNew, Effect: "succeeded", Want: state.RunningUnknown},
		{Name: "xi", Pin: PathNew, RecordToplevel: PathNew, Effect: "succeeded", Want: state.Unreachable},
		{Name: "omicron", Deploy: "done", Running: PathNew, Want: state.PinUnknown},
	},
}

// HostNames returns the scenario's host names, in order.
func (s Scenario) HostNames() []string {
	names := make([]string, len(s.Hosts))
	for i, h := range s.Hosts {
		names[i] = h.Name
	}
	return names
}

func (s Scenario) host(name string) (Host, bool) {
	for _, h := range s.Hosts {
		if h.Name == name {
			return h, true
		}
	}
	return Host{}, false
}

// Handler serves all backends of s.
func (s Scenario) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /pins/{host}", func(w http.ResponseWriter, r *http.Request) {
		h, ok := s.host(r.PathValue("host"))
		if !ok || h.Pin == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, h.Pin)
	})

	api := "/api/repos/" + s.Repo
	mux.HandleFunc("GET "+api+"/builds", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				// a newer push cancelled, and a PR from a branch named main
				{"number": s.BuildNumber + 2, "commit_sha": "ffff", "branch": "main", "pr_number": 7, "status": "building"},
				{"number": s.BuildNumber + 1, "commit_sha": "eeee", "branch": "main", "pr_number": nil, "status": "cancelled"},
				{"number": s.BuildNumber, "commit_sha": s.BuildCommit, "branch": "main", "pr_number": nil, "status": s.BuildStatus, "error": nil},
			},
			"page": 1, "has_next": false,
		})
	})
	mux.HandleFunc("GET "+api+"/builds/{n}", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) || r.PathValue("n") != strconv.Itoa(s.BuildNumber) {
			http.NotFound(w, r)
			return
		}
		effects := []map[string]any{}
		for _, h := range s.Hosts {
			if h.Effect == "" {
				continue
			}
			e := map[string]any{"kind": "push", "name": s.EffectPrefix + h.Name, "status": h.Effect, "error": nil}
			if h.EffectErr != "" {
				e["error"] = h.EffectErr
			}
			effects = append(effects, e,
				// the PR-style check of the same effect, which must be ignored
				map[string]any{"kind": "check", "name": "push." + s.EffectPrefix + h.Name, "status": "failed"})
		}
		writeJSON(w, map[string]any{"build": map[string]any{}, "attributes": []any{}, "effects": effects})
	})
	mux.HandleFunc("GET /repos/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "fake nixbot page for %s\n", r.URL.Path)
	})

	mux.HandleFunc("GET /ntfy/pins/json", func(w http.ResponseWriter, r *http.Request) {
		t := time.Now().Add(-10 * time.Minute).Unix()
		for i, h := range s.Hosts {
			if h.RecordToplevel == "" {
				continue
			}
			commit := h.RecordCommit
			if commit == "" {
				commit = "e7044fbe0a1b2c3d4e5f60718293a4b5c6d7e8f9"
			}
			msg, _ := json.Marshal(map[string]string{"host": h.Name, "commit": commit, "toplevel": h.RecordToplevel})
			line, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("m%d", i), "time": t, "event": "message", "message": string(msg)})
			fmt.Fprintf(w, "%s\n", line)
		}
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.IsAbs() {
			s.comin(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// comin serves http://<host>:<port>/metrics, requested through the proxy.
func (s Scenario) comin(w http.ResponseWriter, r *http.Request) {
	h, ok := s.host(r.URL.Hostname())
	if !ok || h.Deploy == "" {
		http.Error(w, "fake: host unreachable", http.StatusBadGateway)
		return
	}
	if r.URL.Path != "/metrics" {
		http.NotFound(w, r)
		return
	}
	labels := fmt.Sprintf("status=%q", h.Deploy)
	if h.Running != "" {
		labels += fmt.Sprintf(",out_path=%q", h.Running)
	}
	failed := 0
	if h.LastFailed {
		failed = 1
	}
	fmt.Fprintf(w, "comin_deployment_info{%s} 1\ncomin_last_deployment_failed %d\n", labels, failed)
}

func (s Scenario) authorized(r *http.Request) bool {
	return s.Token == "" || r.Header.Get("Authorization") == "Bearer "+s.Token
}

// DashboardFlags returns deploy-dashboard flags that point at s served on
// base (e.g. http://127.0.0.1:8095); comin requests must go through base as
// HTTP_PROXY. tokenFile holds s.Token.
func (s Scenario) DashboardFlags(base, tokenFile string) []string {
	return []string{
		"-hosts=" + strings.Join(s.HostNames(), ","),
		"-niks3-url=" + base,
		"-ntfy-topic-url=" + base + "/ntfy/pins",
		"-nixbot-url=" + base,
		"-nixbot-repo=" + s.Repo,
		"-nixbot-token-file=" + tokenFile,
		"-repo-url=https://github.com/" + strings.SplitN(s.Repo, "/", 2)[1],
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
