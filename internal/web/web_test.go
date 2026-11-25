package web

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
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
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}
