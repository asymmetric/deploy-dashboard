package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asymmetric/deploy-dashboard/internal/pin"
)

func TestPins(t *testing.T) {
	const top = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-x"
	ok := `{"host":"alpha","commit":"abc","toplevel":"` + top + `"}`
	tests := []struct {
		name, token, auth, body string
		want                    int
	}{
		{"no token", "", "", ok, 204},
		{"token", "s3cret", "Bearer s3cret", ok, 204},
		{"missing token", "s3cret", "", ok, 401},
		{"wrong token", "s3cret", "Bearer nope", ok, 401},
		{"unknown host", "", "", `{"host":"zeta","toplevel":"` + top + `"}`, 400},
		{"not a store path", "", "", `{"host":"alpha","toplevel":"/tmp/x"}`, 400},
		{"unknown field", "", "", `{"host":"alpha","toplevel":"` + top + `","x":1}`, 400},
		{"not json", "", "", `host=alpha`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []pin.Record
			h := PinsHandler([]string{"alpha"}, tt.token, func(r pin.Record) error {
				got = append(got, r)
				return nil
			})
			req := httptest.NewRequest("POST", "/pins", strings.NewReader(tt.body))
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want != 204 {
				if len(got) != 0 {
					t.Errorf("stored %+v on a rejected request", got)
				}
				return
			}
			if len(got) != 1 || got[0].Host != "alpha" || got[0].Commit != "abc" || got[0].Toplevel != top || got[0].Time.IsZero() {
				t.Errorf("stored %+v", got)
			}
		})
	}
}
