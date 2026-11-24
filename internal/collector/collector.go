// Package collector polls all sources and keeps the latest snapshot.
package collector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/niks3"
	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
	"github.com/asymmetric/deploy-dashboard/internal/state"
	"github.com/asymmetric/deploy-dashboard/internal/store"
)

// Collector refreshes a state.Snapshot every Interval.
type Collector struct {
	Hosts    []string
	Interval time.Duration
	Timeout  time.Duration
	Niks3    *niks3.Client
	Comin    *comin.Client
	Ntfy     *ntfy.Client
	Store    *store.Store

	mu   sync.RWMutex
	snap state.Snapshot
}

// Snapshot returns the latest snapshot.
func (c *Collector) Snapshot() state.Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap
}

// Run refreshes until ctx is done.
func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		c.refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (c *Collector) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	recs, err := c.Ntfy.Poll(ctx, c.Store.Cursor())
	if err != nil {
		slog.Warn("ntfy poll", "err", err)
	} else if err := c.Store.Add(recs); err != nil {
		slog.Error("store pin records", "err", err)
	}
	latest := c.Store.Latest()

	hosts := make([]state.Host, len(c.Hosts))
	var wg sync.WaitGroup
	for i, name := range c.Hosts {
		wg.Go(func() {
			h := state.Host{Name: name}
			h.Pin, h.PinErr = c.Niks3.Pin(ctx, name)
			h.Comin, h.CominErr = c.Comin.Scrape(ctx, name)
			if h.CominErr == nil {
				h.SeenAt = time.Now()
				if err := c.Store.Observe(name, *h.Comin, h.SeenAt); err != nil {
					slog.Error("store comin status", "host", name, "err", err)
				}
			} else if seen, ok := c.Store.LastSeen(name); ok {
				// offline: show what it last ran
				h.Comin, h.SeenAt = &seen.Status, seen.Time
			}
			if r, ok := latest[name]; ok {
				h.Record = &r
			}
			state.Classify(&h)
			hosts[i] = h
		})
	}
	wg.Wait()

	c.mu.Lock()
	c.snap = state.Snapshot{Hosts: hosts, Updated: time.Now()}
	c.mu.Unlock()
}
