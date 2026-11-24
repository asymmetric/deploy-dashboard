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
	"sync"
	"syscall"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/collector"
	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/niks3"
	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
	"github.com/asymmetric/deploy-dashboard/internal/pin"
	"github.com/asymmetric/deploy-dashboard/internal/store"
	"github.com/asymmetric/deploy-dashboard/internal/web"
)

func main() {
	var (
		listen     = flag.String("listen", "127.0.0.1:8090", "HTTP listen address of the dashboard")
		pinsListen = flag.String("pins-listen", "", "HTTP listen address for POST /pins, where the pin effects post their records (off if empty)")
		pinsToken  = flag.String("pins-token-file", "", "file with the bearer token POST /pins requires (none if empty)")
		hosts      = flag.String("hosts", "", "comma-separated gitops hosts")
		niks3URL   = flag.String("niks3-url", "", "niks3 server URL (required)")
		ntfyURL    = flag.String("ntfy-topic-url", "", "ntfy topic to poll for pin records (off if empty)")
		cominPort  = flag.Int("comin-port", 4243, "comin exporter port on each host")
		stateFile  = flag.String("state-file", "state.json", "file for persisted pin records")
		interval   = flag.Duration("interval", 15*time.Second, "refresh interval")
	)
	flag.Parse()

	for name, v := range map[string]string{"hosts": *hosts, "niks3-url": *niks3URL} {
		if v == "" {
			slog.Error("-" + name + " is required")
			os.Exit(2)
		}
	}
	if *pinsListen == "" && *ntfyURL == "" {
		slog.Warn("neither -pins-listen nor -ntfy-topic-url is set: no pin records, so no commits and no detection of pins moved by hand")
	}
	var token string
	if *pinsToken != "" {
		b, err := os.ReadFile(*pinsToken)
		if err != nil {
			slog.Error("read pins token", "err", err)
			os.Exit(1)
		}
		token = strings.TrimSpace(string(b))
		if token == "" {
			slog.Error("pins token file is empty", "file", *pinsToken)
			os.Exit(1)
		}
	}
	st, err := store.Open(*stateFile)
	if err != nil {
		slog.Error("open state", "err", err)
		os.Exit(1)
	}

	hostList := strings.Split(*hosts, ",")
	httpc := &http.Client{Timeout: 5 * time.Second}
	col := &collector.Collector{
		Hosts:    hostList,
		Interval: *interval,
		Timeout:  *interval,
		Niks3:    &niks3.Client{BaseURL: *niks3URL, HTTP: httpc},
		Comin:    &comin.Client{Port: *cominPort, HTTP: httpc},
		Store:    st,
	}
	if *ntfyURL != "" {
		col.Ntfy = &ntfy.Client{TopicURL: *ntfyURL, HTTP: httpc}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go col.Run(ctx)

	srvs := []*http.Server{{Addr: *listen, Handler: web.Handler(col.Snapshot), ReadHeaderTimeout: 5 * time.Second}}
	if *pinsListen != "" {
		add := func(r pin.Record) error { return st.Add(r) }
		srvs = append(srvs, &http.Server{Addr: *pinsListen, Handler: web.PinsHandler(hostList, token, add), ReadHeaderTimeout: 5 * time.Second})
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, srv := range srvs {
			srv.Shutdown(shutdownCtx)
		}
	}()

	var wg sync.WaitGroup
	for _, srv := range srvs {
		wg.Go(func() {
			slog.Info("listening", "addr", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("serve", "addr", srv.Addr, "err", err)
				os.Exit(1)
			}
		})
	}
	wg.Wait()
}
