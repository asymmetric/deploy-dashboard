// Command fake-backends serves fake niks3, comin, nixbot and ntfy backends
// (internal/fake) for a local look at deploy-dashboard. `nix run .#demo`
// starts it together with the dashboard.
//
// Run the dashboard with HTTP_PROXY pointing at it, so comin scrapes reach
// the fake, and with the flags -print-flags prints.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/fake"
)

func main() {
	var (
		listen     = flag.String("listen", "127.0.0.1:8095", "listen address; must be a loopback IP, which Go never proxies")
		printFlags = flag.Bool("print-flags", false, "print the deploy-dashboard flags for -listen and -token-file, and exit")
		tokenFile  = flag.String("token-file", "nixbot-token", "file the nixbot token is written to, for -nixbot-token-file")
	)
	flag.Parse()
	sc := fake.Default
	base := "http://" + *listen

	if *printFlags {
		fmt.Println(strings.Join(sc.DashboardFlags(base, *tokenFile), " "))
		return
	}
	if err := os.WriteFile(*tokenFile, []byte(sc.Token+"\n"), 0o600); err != nil {
		slog.Error("write token", "err", err)
		os.Exit(1)
	}
	slog.Info("serving fake backends", "addr", *listen, "hosts", sc.HostNames())
	srv := &http.Server{Addr: *listen, Handler: sc.Handler(), ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
