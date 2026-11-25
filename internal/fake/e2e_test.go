package fake_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/collector"
	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/fake"
	"github.com/asymmetric/deploy-dashboard/internal/niks3"
	"github.com/asymmetric/deploy-dashboard/internal/nixbot"
	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
	"github.com/asymmetric/deploy-dashboard/internal/store"
	"github.com/asymmetric/deploy-dashboard/internal/web"
)

// TestEndToEnd runs the collector against fake.Default and checks every
// host's state and the rendered page.
func TestEndToEnd(t *testing.T) {
	sc := fake.Default
	srv := httptest.NewServer(sc.Handler())
	defer srv.Close()
	proxy, _ := url.Parse(srv.URL)

	st, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	direct := srv.Client()
	col := &collector.Collector{
		Hosts:   sc.HostNames(),
		Timeout: 10 * time.Second,
		Niks3:   &niks3.Client{BaseURL: srv.URL, HTTP: direct},
		// comin exporters are reached through the fake as a proxy
		Comin:        &comin.Client{Port: 4243, HTTP: &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}},
		Ntfy:         &ntfy.Client{TopicURL: srv.URL + "/ntfy/pins", HTTP: direct},
		Store:        st,
		Nixbot:       &nixbot.Client{BaseURL: srv.URL, Repo: sc.Repo, Token: sc.Token, HTTP: direct},
		NixbotBranch: "main",
		NixbotEffect: sc.EffectPrefix + "{host}",
	}
	col.Refresh(context.Background())

	snap := col.Snapshot()
	for i, want := range sc.Hosts {
		got := snap.Hosts[i]
		if got.Name != want.Name || got.Kind != want.Want {
			t.Errorf("%s: got %q, want %q (pin err %v, comin err %v, ci err %v)", want.Name, got.Kind, want.Want, got.PinErr, got.CominErr, got.CIErr)
		}
		if got.CIErr != nil {
			t.Errorf("%s: nixbot: %v", want.Name, got.CIErr)
		}
		if want.Effect != "" && (got.CI == nil || got.CI.Status() != want.Effect) {
			t.Errorf("%s: CI = %+v, want effect status %q", want.Name, got.CI, want.Effect)
		}
	}

	rec := httptest.NewRecorder()
	web.Handler(col.Snapshot, web.Repo{URL: "https://github.com/owner/dotfiles"}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`<td class="bad">build failed</td>`,
		`<td class="pending">building</td>`,
		`href="` + srv.URL + `/repos/github/owner/dotfiles/builds/128/logs/effect:default.pin-qirex"`,
		`>failed</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}
