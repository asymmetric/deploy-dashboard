package state

import (
	"errors"
	"testing"

	"github.com/asymmetric/deploy-dashboard/internal/comin"
	"github.com/asymmetric/deploy-dashboard/internal/nixbot"
	"github.com/asymmetric/deploy-dashboard/internal/pin"
)

func TestClassify(t *testing.T) {
	const a, b = "/nix/store/a", "/nix/store/b"
	errX := errors.New("x")
	synced := comin.Status{StorePath: a, DeployStatus: comin.StatusDone}
	ci := func(build, effect string) *CI {
		c := &CI{Build: nixbot.Build{Status: build}}
		if effect != "" {
			c.Effect = &nixbot.Effect{Kind: nixbot.KindPush, Status: effect}
		}
		return c
	}
	for _, tc := range []struct {
		name string
		h    Host
		want Kind
	}{
		{"pin error", Host{PinErr: errX}, PinUnknown},
		{"by hand", Host{Pin: a, Record: &pin.Record{Toplevel: b}, Comin: &comin.Status{StorePath: a}}, PinnedByHand},
		{"unreachable, never seen", Host{Pin: a, CominErr: errX}, Unreachable},
		{"offline, in sync", Host{Pin: a, CominErr: errX, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}}, Offline},
		{"offline, behind", Host{Pin: a, CominErr: errX, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusDone}}, OfflineBehind},
		{"offline, old comin", Host{Pin: a, CominErr: errX, Comin: &comin.Status{DeployStatus: comin.StatusDone}}, Offline},
		{"in sync", Host{Pin: a, Record: &pin.Record{Toplevel: a}, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusDone}}, InSync},
		{"deploying", Host{Pin: a, Comin: &comin.Status{StorePath: a, DeployStatus: comin.StatusRunning}}, Deploying},
		{"failing", Host{Pin: a, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusFailed}}, DeployFailing},
		{"failing, old comin", Host{Pin: a, Comin: &comin.Status{LastDeploymentFailed: true}}, DeployFailing},
		{"old comin", Host{Pin: a, Comin: &comin.Status{DeployStatus: comin.StatusDone}}, RunningUnknown},
		{"pending", Host{Pin: a, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusDone}}, PendingDeploy},
		{"effect failed", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildFailed, nixbot.EffectFailed)}, BuildFailed},
		{"build failed before effects", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildFailed, "")}, BuildFailed},
		{"effect skipped", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildFailed, nixbot.EffectSkipped)}, BuildFailed},
		{"other host's effect failed", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildFailed, nixbot.EffectSucceeded)}, InSync},
		{"build failed, offline", Host{Pin: a, CominErr: errX, Comin: &synced, CI: ci(nixbot.BuildFailed, "")}, BuildFailed},
		{"build failed beats by hand", Host{Pin: a, Record: &pin.Record{Toplevel: b}, Comin: &synced, CI: ci(nixbot.BuildFailed, "")}, BuildFailed},
		{"evaluating", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildEvaluating, "")}, Building},
		{"effect running", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildSucceeded, nixbot.EffectRunning)}, Building},
		{"building, pin waiting", Host{Pin: a, Comin: &comin.Status{StorePath: b, DeployStatus: comin.StatusDone}, CI: ci(nixbot.BuildBuilding, "")}, PendingDeploy},
		{"built", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildSucceeded, nixbot.EffectSucceeded)}, InSync},
		{"cancelled effect", Host{Pin: a, Comin: &synced, CI: ci(nixbot.BuildSucceeded, nixbot.EffectCancelled)}, InSync},
	} {
		Classify(&tc.h)
		if tc.h.Kind != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.h.Kind, tc.want)
		}
	}
}
