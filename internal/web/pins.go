package web

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/pin"
)

// maxPinBody bounds a POST /pins body; a record is well under 1 KiB.
const maxPinBody = 64 << 10

// PinsHandler accepts the records the pin effects POST after
// `niks3 push --pin`:
//
//	curl -fsS -d '{"host":"…","commit":"…","toplevel":"/nix/store/…"}' <url>/pins
//
// Records for hosts not in hosts are rejected. If token isn't empty, requests
// must carry "Authorization: Bearer <token>". The record's time is when it was
// received.
func PinsHandler(hosts []string, token string, add func(pin.Record) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /pins", func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		var rec pin.Record
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPinBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rec); err != nil {
			http.Error(w, "bad record: "+err.Error(), http.StatusBadRequest)
			return
		}
		switch {
		case !slices.Contains(hosts, rec.Host):
			http.Error(w, "unknown host "+rec.Host, http.StatusBadRequest)
			return
		case !strings.HasPrefix(rec.Toplevel, "/nix/store/"):
			http.Error(w, "toplevel is not a store path", http.StatusBadRequest)
			return
		}
		rec.Time = time.Now()
		if err := add(rec); err != nil {
			slog.Error("store pin record", "host", rec.Host, "err", err)
			http.Error(w, "could not store the record", http.StatusInternalServerError)
			return
		}
		slog.Info("pin record", "host", rec.Host, "commit", rec.Commit, "toplevel", rec.Toplevel)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
