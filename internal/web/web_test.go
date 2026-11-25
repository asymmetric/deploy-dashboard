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
	build := nixbot.Build{Number: 42, CommitSHA: "fedcba9876543210", Status: nixbot.BuildFailed}
	hosts := []state.Host{
		{Name: "up", Pin: a, Record: &pin.Record{Commit: "0123456789abcdef", Toplevel: a}, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, SeenAt: time.Now()},
		{Name: "laptop", Pin: a, CominErr: errDown, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, SeenAt: time.Now().Add(-3 * time.Hour)},
		{Name: "never", Pin: a, CominErr: errDown},
		{Name: "broken", Pin: a, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, CI: &state.CI{
			Build:  build,
			Effect: &nixbot.Effect{Kind: nixbot.KindPush, Name: "default.pin-broken", Status: nixbot.EffectFailed, Error: new("exit 1")},
			LogURL: "https://nixbot.example/repos/github/o/r/builds/42/logs/effect:default.pin-broken",
		}},
		{Name: "before-effects", Pin: a, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}, CI: &state.CI{Build: build}},
	}
	for i := range hosts {
		state.Classify(&hosts[i])
	}
	snap := state.Snapshot{
		Hosts:   hosts,
		Build:   &state.Build{Build: build, Branch: "main", URL: "https://nixbot.example/repos/github/o/r/builds/42"},
		Updated: time.Now(),
	}

	body := render(t, snap)
	for _, want := range []string{
		`newest build of <code>main</code>: <a href="https://nixbot.example/repos/github/o/r/builds/42">#42</a> <a href="https://example.com/o/r/commit/fedcba9876543210"><code>fedcba98</code></a> <span class="bad">failed</span>`,
		`<td><a class="bad" href="https://nixbot.example/repos/github/o/r/builds/42/logs/effect:default.pin-broken" title="exit 1">failed</a></td>`,
		`<td><span class="muted">—</span></td>`,
		`<td class="ok">in sync</td>`,
		`<td class="muted">offline</td>`,
		`last seen 3h0m0s ago`,
		`<td class="bad">unreachable</td>`,
		`<span class="err">connect: no route to host</span>`,
		`<a href="https://example.com/o/r/commit/0123456789abcdef"><code>01234567</code></a>`,
		`<td class="bad">build failed</td>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
	// the build is shown once, not per host
	if n := strings.Count(body, ">#42<"); n != 1 {
		t.Errorf("build number shown %d times, want 1", n)
	}
}

func TestRenderBuildErr(t *testing.T) {
	body := render(t, state.Snapshot{BuildErr: errors.New("GET x: 404 Not Found"), Updated: time.Now()})
	if want := `newest build: <span class="err">GET x: 404 Not Found</span>`; !strings.Contains(body, want) {
		t.Errorf("body lacks %q", want)
	}
}

func render(t *testing.T, snap state.Snapshot) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler(func() state.Snapshot { return snap }, Repo{URL: "https://example.com/o/r/"}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	return rec.Body.String()
}
