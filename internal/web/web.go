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
	"ago": func(t time.Time) string {
		return time.Since(t).Round(time.Second).String()
	},
	"cls": func(k state.Kind) string {
		switch k {
		case state.InSync:
			return "ok"
		case state.PendingDeploy, state.Deploying, state.OfflineBehind:
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

// Handler serves the dashboard from snap.
func Handler(snap func() state.Snapshot) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.ExecuteTemplate(w, "index.html", snap()); err != nil {
			slog.Error("render", "err", err)
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	return mux
}
