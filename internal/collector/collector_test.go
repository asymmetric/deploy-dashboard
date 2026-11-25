package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric/deploy-dashboard/internal/nixbot"
)

func TestCI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repos/github/o/r/builds", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"number":5,"commit_sha":"c5","branch":"main","pr_number":null,"status":"succeeded"}]}`))
	})
	mux.HandleFunc("GET /api/repos/github/o/r/builds/5", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"effects":[
			{"kind":"check","name":"push.default.pin-a","status":"failed"},
			{"kind":"push","name":"default.pin-a","status":"running"}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Collector{
		Nixbot:       &nixbot.Client{BaseURL: srv.URL, Repo: "github/o/r", HTTP: srv.Client()},
		NixbotBranch: "main",
		NixbotEffect: "default.pin-{host}",
	}
	build, effects, err := c.build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if build.Number != 5 || build.Branch != "main" || build.URL != srv.URL+"/repos/github/o/r/builds/5" {
		t.Errorf("build = %+v", build)
	}

	a := c.hostCI(build, effects, "a")
	if a.Effect == nil || a.Effect.Kind != nixbot.KindPush || a.Effect.Status != nixbot.EffectRunning {
		t.Errorf("a: effect = %+v, want the push effect", a.Effect)
	}
	if want := srv.URL + "/repos/github/o/r/builds/5/logs/effect:default.pin-a"; a.LogURL != want {
		t.Errorf("a: LogURL = %q, want %q", a.LogURL, want)
	}

	b := c.hostCI(build, effects, "b")
	if b.Build.Number != 5 || b.Effect != nil || b.LogURL != "" {
		t.Errorf("b: %+v, want build 5 without effect", b)
	}
}

func TestCIOff(t *testing.T) {
	c := &Collector{}
	build, effects, err := c.build(context.Background())
	if build != nil || effects != nil || err != nil {
		t.Errorf("got %v, %v, %v; want nothing without nixbot", build, effects, err)
	}
	if ci := c.hostCI(nil, nil, "a"); ci != nil {
		t.Errorf("hostCI = %+v, want nil", ci)
	}
}
