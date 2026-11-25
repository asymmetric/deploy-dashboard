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
	ci := c.ci(context.Background())

	a, err := ci("a")
	if err != nil {
		t.Fatal(err)
	}
	if a.Effect == nil || a.Effect.Kind != nixbot.KindPush || a.Effect.Status != nixbot.EffectRunning {
		t.Errorf("a: effect = %+v, want the push effect", a.Effect)
	}
	if want := srv.URL + "/repos/github/o/r/builds/5/logs/effect:default.pin-a"; a.LogURL != want {
		t.Errorf("a: LogURL = %q, want %q", a.LogURL, want)
	}

	b, err := ci("b")
	if err != nil {
		t.Fatal(err)
	}
	if b.Build.Number != 5 || b.Effect != nil || b.LogURL != "" {
		t.Errorf("b: %+v, want build 5 without effect", b)
	}
}

func TestCIOff(t *testing.T) {
	ci, err := (&Collector{}).ci(context.Background())("a")
	if ci != nil || err != nil {
		t.Errorf("got %v, %v; want nothing without nixbot", ci, err)
	}
}
