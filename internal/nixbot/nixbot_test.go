package nixbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repos/github/o/r/builds", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("branch"); got != "main" {
			t.Errorf("branch = %q", got)
		}
		// newest first, as nixbot returns them
		w.Write([]byte(`{"items":[
			{"number":9,"commit_sha":"c9","branch":"main","pr_number":3,"status":"building","error":null},
			{"number":8,"commit_sha":"c8","branch":"main","pr_number":null,"status":"cancelled","error":null},
			{"number":7,"commit_sha":"c7","branch":"main","pr_number":null,"status":"failed","error":null,"created_at":"2026-11-25T10:00:00.123456+00:00","extra":1}
		],"page":1,"has_next":false}`))
	})
	mux.HandleFunc("GET /api/repos/github/o/r/builds/7", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"build":{},"attributes":[],"effects":[
			{"id":1,"kind":"push","name":"default.pin-a","status":"failed","error":"boom"},
			{"id":2,"kind":"push","name":"default.pin-b","status":"succeeded","error":null}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/", Repo: "github/o/r", Token: "tok", HTTP: srv.Client()}
	b, effects, ok, err := c.Latest(context.Background(), "main")
	if err != nil || !ok {
		t.Fatalf("Latest: ok=%v err=%v", ok, err)
	}
	if b.Number != 7 || b.CommitSHA != "c7" || b.Status != BuildFailed {
		t.Errorf("build = %+v", b)
	}
	if len(effects) != 2 || effects[0].Name != "default.pin-a" || *effects[0].Error != "boom" || effects[1].Error != nil {
		t.Errorf("effects = %+v", effects)
	}
	if got, want := c.EffectLogURL(7, "default.pin-a"), srv.URL+"/repos/github/o/r/builds/7/logs/effect:default.pin-a"; got != want {
		t.Errorf("EffectLogURL = %q, want %q", got, want)
	}
}

func TestLatestNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"page":1,"has_next":false}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Repo: "github/o/r", HTTP: srv.Client()}
	if _, _, ok, err := c.Latest(context.Background(), "main"); ok || err != nil {
		t.Errorf("ok=%v err=%v, want no build and no error", ok, err)
	}
}

func TestNotVisible(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Repo: "github/o/r", HTTP: srv.Client()}
	if _, _, _, err := c.Latest(context.Background(), "main"); err == nil {
		t.Error("want an error for 404")
	}
}
