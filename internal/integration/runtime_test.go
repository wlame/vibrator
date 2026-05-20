package integration

import (
	"context"
	"testing"
)

func TestExternalRuntime_StatusAlwaysNotRunning(t *testing.T) {
	r := &ExternalRuntime{Instructions: "set it up yourself"}
	got, err := r.Status(context.Background())
	if err != nil {
		t.Errorf("Status: %v", err)
	}
	if got.Running {
		t.Error("ExternalRuntime.Status.Running = true (should never report owning the process)")
	}
}

func TestExternalRuntime_LifecycleIsNoOp(t *testing.T) {
	r := &ExternalRuntime{}
	if err := r.Start(context.Background()); err != nil {
		t.Errorf("Start: %v", err)
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
	logs, err := r.Logs(context.Background(), 1024)
	if err != nil {
		t.Errorf("Logs: %v", err)
	}
	if logs != "" {
		t.Errorf("Logs = %q, want empty for ExternalRuntime", logs)
	}
}

func TestExternalRuntime_KindAndLabel(t *testing.T) {
	r := &ExternalRuntime{}
	if r.Kind() != "external" {
		t.Errorf("Kind = %q, want external", r.Kind())
	}
	if r.Label() == "" {
		t.Error("Label is empty")
	}
}
