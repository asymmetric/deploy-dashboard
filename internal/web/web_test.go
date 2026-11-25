package web

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/nixbot"
	"github.com/asymmetric/deploy-dashboard/internal/pin"
	"github.com/asymmetric/deploy-dashboard/internal/state"
)

func TestRender(t *testing.T) {
	const a = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-x"
	errDown := errors.New("connect: no route to host")
	hosts := []state.Host{
		{Name: "up", Pin: a, Record: &pin.Record{Commit: "0123456789abcdef", Toplevel: a}, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, SeenAt: time.Now()},
		{Name: "laptop", Pin: a, CominErr: errDown, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, SeenAt: time.Now().Add(-3 * time.Hour)},
		{Name: "never", Pin: a, CominErr: errDown},
		{Name: "broken", Pin: a, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, CI: &state.CI{
			Build:    nixbot.Build{Number: 42, CommitSHA: "fedcba9876543210", Status: nixbot.BuildFailed},
			Effect:   &nixbot.Effect{Kind: nixbot.KindPush, Name: "default.pin-broken", Status: nixbot.EffectFailed, Error: new("exit 1")},
			BuildURL: "https://nixbot.example/repos/github/o/r/builds/42",
			LogURL:   "https://nixbot.example/repos/github/o/r/builds/42/logs/effect:default.pin-broken",
		}},
		{Name: "noci", Pin: a, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, CIErr: errors.New("GET x: 404 Not Found")},
	}
	for i := range hosts {
		state.Classify(&hosts[i])
	}
	snap := state.Snapshot{Hosts: hosts, Updated: time.Now()}

	rec := httptest.NewRecorder()
	Handler(func() state.Snapshot { return snap }, Repo{URL: "https://example.com/o/r/"}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<td class="ok">in sync</td>`,
		`<td class="muted">offline</td>`,
		`last seen 3h0m0s ago`,
		`<td class="bad">unreachable</td>`,
		`<span class="err">connect: no route to host</span>`,
		`<a href="https://example.com/o/r/commit/0123456789abcdef"><code>01234567</code></a>`,
		`<td class="bad">build failed</td>`,
		`<a href="https://nixbot.example/repos/github/o/r/builds/42">#42</a> <a href="https://example.com/o/r/commit/fedcba9876543210"><code>fedcba98</code></a> <a class="bad" href="https://nixbot.example/repos/github/o/r/builds/42/logs/effect:default.pin-broken" title="exit 1">failed</a>`,
		`<span class="err">GET x: 404 Not Found</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}
