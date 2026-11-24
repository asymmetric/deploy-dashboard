// Package state joins pins, comin status and pin records into a per-host state.
package state

import (
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/pin"
)

// Kind is a host's derived deployment state.
type Kind string

const (
	InSync         Kind = "in sync"
	Deploying      Kind = "deploying"
	PendingDeploy  Kind = "pinned, not yet deployed"
	DeployFailing  Kind = "deploy failing"
	PinnedByHand   Kind = "pinned by hand"
	Unreachable    Kind = "unreachable"
	Offline        Kind = "offline"
	OfflineBehind  Kind = "offline, pin waiting"
	PinUnknown     Kind = "pin unknown"
	RunningUnknown Kind = "running path unknown"
)

// Host is one row of the dashboard.
type Host struct {
	Name string
	Kind Kind

	Pin    string
	PinErr error

	// Comin is the current status, or the last one seen if CominErr is set
	// (nil if the host was never reached).
	Comin    *comin.Status
	CominErr error
	// SeenAt is when Comin was scraped.
	SeenAt time.Time

	// Record is the last pin published by CI for this host, if any.
	Record *pin.Record
}

// Snapshot is the state of all hosts at one point in time.
type Snapshot struct {
	Hosts   []Host
	Updated time.Time
}

// Classify sets h.Kind from the other fields.
func Classify(h *Host) {
	switch {
	case h.PinErr != nil:
		h.Kind = PinUnknown
	case h.Record != nil && h.Record.Toplevel != h.Pin:
		// CI's last pin differs from the current one: moved by hand, e.g. a rollback.
		h.Kind = PinnedByHand
	case h.CominErr != nil && h.Comin == nil:
		h.Kind = Unreachable
	case h.CominErr != nil && h.Comin.StorePath != "" && h.Comin.StorePath != h.Pin:
		// e.g. a laptop that is off: comin deploys the pin once it's back
		h.Kind = OfflineBehind
	case h.CominErr != nil:
		h.Kind = Offline
	case h.Comin.LastDeploymentFailed || h.Comin.DeployStatus == comin.StatusFailed:
		h.Kind = DeployFailing
	case h.Comin.StorePath == "":
		// comin without the out_path label: only failures are visible.
		h.Kind = RunningUnknown
	case h.Comin.StorePath != h.Pin:
		h.Kind = PendingDeploy
	case h.Comin.DeployStatus == comin.StatusDone:
		h.Kind = InSync
	default:
		// deploying the pin right now (init or running)
		h.Kind = Deploying
	}
}
