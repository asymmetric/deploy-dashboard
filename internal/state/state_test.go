package state

import (
	"errors"
	"testing"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/ntfy"
)

func TestClassify(t *testing.T) {
	const a, b = "/nix/store/a", "/nix/store/b"
	errX := errors.New("x")
	for _, tc := range []struct {
		name string
		h    Host
		want Kind
	}{
		{"pin error", Host{PinErr: errX}, PinUnknown},
		{"by hand", Host{Pin: a, Record: &ntfy.PinRecord{Toplevel: b}, Comin: &comin.Status{StorePath: a}}, PinnedByHand},
		{"unreachable, never seen", Host{Pin: a, CominErr: errX}, Unreachable},
		{"offline, in sync", Host{Pin: a, CominErr: errX, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}}, Offline},
		{"offline, behind", Host{Pin: a, CominErr: errX, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusDone}}, OfflineBehind},
		{"offline, old comin", Host{Pin: a, CominErr: errX, Comin: &comin.Status{DeployStatus: comin.StatusDone}}, Offline},
		{"in sync", Host{Pin: a, Record: &ntfy.PinRecord{Toplevel: a}, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}}, InSync},
		{"deploying", Host{Pin: a, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusRunning}}, Deploying},
		{"failing", Host{Pin: a, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusFailed}}, DeployFailing},
		{"failing, old comin", Host{Pin: a, Comin: &comin.Status{LastDeploymentFailed: true}}, DeployFailing},
		{"old comin", Host{Pin: a, Comin: &comin.Status{DeployStatus: comin.StatusDone}}, RunningUnknown},
		{"pending", Host{Pin: a, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusDone}}, PendingDeploy},
	} {
		Classify(&tc.h)
		if tc.h.Kind != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.h.Kind, tc.want)
		}
	}
}
