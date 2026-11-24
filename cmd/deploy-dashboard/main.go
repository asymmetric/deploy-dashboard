// Command deploy-dashboard shows, per host, what CI pinned and what comin runs.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/collector"
	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/niks3"
	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
	"github.com/asymmetric/deploy-dashboard/internal/store"
	"github.com/asymmetric/deploy-dashboard/internal/web"
)

func main() {
	var (
		listen    = flag.String("listen", "127.0.0.1:8090", "HTTP listen address")
		hosts     = flag.String("hosts", "", "comma-separated gitops hosts")
		niks3URL  = flag.String("niks3-url", "", "niks3 server URL (required)")
		ntfyURL   = flag.String("ntfy-topic-url", "", "ntfy topic the pin effects publish to (required)")
		cominPort = flag.Int("comin-port", 4243, "comin exporter port on each host")
		stateFile = flag.String("state-file", "state.json", "file for persisted pin records")
		interval  = flag.Duration("interval", 15*time.Second, "refresh interval")
	)
	flag.Parse()

	for name, v := range map[string]string{"hosts": *hosts, "niks3-url": *niks3URL, "ntfy-topic-url": *ntfyURL} {
		if v == "" {
			slog.Error("-" + name + " is required")
			os.Exit(2)
		}
	}
	st, err := store.Open(*stateFile)
	if err != nil {
		slog.Error("open state", "err", err)
		os.Exit(1)
	}

	httpc := &http.Client{Timeout: 5 * time.Second}
	col := &collector.Collector{
		Hosts:    strings.Split(*hosts, ","),
		Interval: *interval,
		Timeout:  *interval,
		Niks3:    &niks3.Client{BaseURL: *niks3URL, HTTP: httpc},
		Comin:    &comin.Client{Port: *cominPort, HTTP: httpc},
		Ntfy:     &ntfy.Client{TopicURL: *ntfyURL, HTTP: httpc},
		Store:    st,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go col.Run(ctx)

	srv := &http.Server{Addr: *listen, Handler: web.Handler(col.Snapshot), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	slog.Info("listening", "addr", *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
