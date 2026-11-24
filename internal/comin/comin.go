// Package comin scrapes comin's Prometheus exporter on a host.
package comin

import (
	"context"
	"fmt"
	"net/http"
)

// Metrics from comin's internal/prometheus/prometheus.go. The out_path label
// isn't upstream: it needs comin with the out_path patch (see README.md).
const (
	metricDeployment       = "comin_deployment_info"
	metricDeploymentFailed = "comin_last_deployment_failed"
	labelStatus            = "status"
	labelOutPath           = "out_path"
)

// Deployment statuses (comin internal/store/deployment.go StatusToString).
const (
	StatusInit    = "init"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Status is the deployment state comin reports.
type Status struct {
	// StorePath is the toplevel of comin's last deployment; empty if comin
	// doesn't export out_path.
	StorePath string `json:"store_path"`
	// DeployStatus is that deployment's status: init, running, done or failed.
	DeployStatus string `json:"deploy_status"`
	// LastDeploymentFailed is comin_last_deployment_failed.
	LastDeploymentFailed bool `json:"last_deployment_failed"`
}

// Client scrapes http://<host>:<Port>/metrics.
type Client struct {
	Port int
	HTTP *http.Client
}

// Scrape fetches and interprets the exporter's metrics on host.
func (c *Client) Scrape(ctx context.Context, host string) (*Status, error) {
	url := fmt.Sprintf("http://%s:%d/metrics", host, c.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	samples, err := Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	st := &Status{}
	for _, s := range samples {
		switch s.Name {
		case metricDeployment:
			if s.Value == 1 {
				st.StorePath = s.Labels[labelOutPath]
				st.DeployStatus = s.Labels[labelStatus]
			}
		case metricDeploymentFailed:
			st.LastDeploymentFailed = s.Value == 1
		}
	}
	return st, nil
}
