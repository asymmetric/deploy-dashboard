// Package web serves the dashboard page.
package web

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/nixbot"
	"github.com/asymmetric/deploy-dashboard/internal/state"
)

//go:embed templates/*.html
var templates embed.FS

var funcs = template.FuncMap{
	// short turns /nix/store/<hash>-<name> into <hash[:8]>…-<name>.
	"short": func(p string) string {
		base := path.Base(p)
		hash, name, ok := strings.Cut(base, "-")
		if !ok || len(hash) < 8 {
			return base
		}
		return hash[:8] + "…-" + name
	},
	// ciCls colors a nixbot build or effect status.
	"ciCls": func(status string) string {
		switch status {
		case nixbot.BuildSucceeded:
			return "ok"
		case nixbot.BuildFailed:
			return "bad"
		case nixbot.BuildCancelled, nixbot.EffectSkipped:
			return "muted"
		default:
			return "pending"
		}
	},
	"ago": func(t time.Time) string {
		return time.Since(t).Round(time.Second).String()
	},
	"cls": func(k state.Kind) string {
		switch k {
		case state.InSync:
			return "ok"
		case state.PendingDeploy, state.Deploying, state.OfflineBehind, state.Building:
			return "pending"
		case state.Offline:
			// expected for laptops; not an error
			return "muted"
		case state.PinnedByHand, state.RunningUnknown:
			return "warn"
		default:
			return "bad"
		}
	},
}

var page = template.Must(template.New("").Funcs(funcs).ParseFS(templates, "templates/*.html"))

// Repo is the config repository the pins are built from, used for links.
// The URL is a GitHub-style web URL, e.g. https://github.com/owner/dotfiles;
// an empty URL disables the links.
type Repo struct {
	URL string
}

// CommitURL links to commit sha.
func (r Repo) CommitURL(sha string) string {
	return strings.TrimRight(r.URL, "/") + "/commit/" + sha
}

type pageData struct {
	state.Snapshot
	Repo Repo
}

// Handler serves the dashboard from snap, linking commits to repo.
func Handler(snap func() state.Snapshot, repo Repo) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.ExecuteTemplate(w, "index.html", pageData{snap(), repo}); err != nil {
			slog.Error("render", "err", err)
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	return mux
}
