// Package state joins pins, comin status and pin records into a per-host state.
package state

import (
	"time"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/nixbot"
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
	BuildFailed    Kind = "build failed"
	Building       Kind = "building"
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

	// CI is Snapshot.Build as it concerns this host, nil if that is nil.
	CI *CI
}

// Build is the newest build of the branch that pins the hosts. It is the
// same for all hosts; each host's part of it is in Host.CI.
type Build struct {
	nixbot.Build
	Branch string
	URL    string
}

// CI is a default-branch build and this host's pin effect in it.
type CI struct {
	Build nixbot.Build
	// Effect is the host's pin effect, nil if the build has none (yet):
	// effects run only after the build succeeded.
	Effect *nixbot.Effect
	// LogURL is the effect's log, empty if Effect is nil.
	LogURL string
}

// Failed reports whether the build won't pin a new toplevel for the host.
func (c *CI) Failed() bool {
	if c.Effect != nil && c.Effect.Status == nixbot.EffectFailed {
		return true
	}
	// no effect run: the build failed before effects (eval, other
	// attributes), or the effect was skipped (or left pending) because one
	// it is after failed
	return (c.Effect == nil || c.Effect.Status == nixbot.EffectSkipped || c.Effect.Status == nixbot.EffectPending) &&
		c.Build.Status == nixbot.BuildFailed
}

// Running reports whether the build may still pin a new toplevel. Once the
// host's effect exists, it decides: effects finish while the build is still
// running other ones.
func (c *CI) Running() bool {
	if c.Effect != nil {
		return c.Effect.Status == nixbot.EffectPending || c.Effect.Status == nixbot.EffectRunning
	}
	return c.Build.Running()
}

// StatusUnchanged is CI.Status for an effect that succeeded without running.
const StatusUnchanged = "unchanged"

// Status is a short description of the build's state for the host: its
// effect's status, or else the build's.
func (c *CI) Status() string {
	if c.Effect != nil {
		if c.Effect.Unchanged() {
			return StatusUnchanged
		}
		return c.Effect.Status
	}
	return c.Build.Status
}

// Error is the effect's error, or else the build's, if any.
func (c *CI) Error() string {
	if c.Effect != nil && c.Effect.Error != nil {
		return *c.Effect.Error
	}
	if c.Build.Error != nil {
		return *c.Build.Error
	}
	return ""
}

// Title explains Status on hover: the error, or why an effect didn't run.
func (c *CI) Title() string {
	if c.Effect != nil && c.Effect.Unchanged() {
		return "not run: its inputs (the toplevel) are those of its last successful run, so the pin is current"
	}
	return c.Error()
}

// Snapshot is the state of all hosts at one point in time.
type Snapshot struct {
	Hosts []Host
	// Build is the newest build, nil if nixbot isn't configured, has no
	// such build, or BuildErr is set.
	Build    *Build
	BuildErr error
	Updated  time.Time
}

// Classify sets h.Kind from the other fields.
func Classify(h *Host) {
	switch {
	case h.PinErr != nil:
		h.Kind = PinUnknown
	case h.CI != nil && h.CI.Failed():
		// main doesn't reach this host, whatever it runs now
		h.Kind = BuildFailed
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
	case h.CI != nil && h.CI.Running():
		// running the pin, but CI may be about to move it
		h.Kind = Building
	case h.Comin.DeployStatus == comin.StatusDone:
		h.Kind = InSync
	default:
		// deploying the pin right now (init or running)
		h.Kind = Deploying
	}
}
