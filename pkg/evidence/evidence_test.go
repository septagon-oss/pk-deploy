package evidence

import (
	"context"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

func TestMemoryRecorderStoresDefensiveCopies(t *testing.T) {
	t.Parallel()

	recorder := NewMemoryRecorder()
	bundle := Bundle{
		ID:            "bundle-1",
		JobID:         "job-1",
		PlanID:        "plan-1",
		ApplicationID: "app",
		EnvironmentID: "staging",
		Status:        deploy.StatusSucceeded,
		StartedAt:     time.Now(),
		FinishedAt:    time.Now(),
		Artifacts: []deploy.Artifact{{
			ID:     "image",
			Kind:   "oci-image",
			Ref:    "registry.example.com/app:v1",
			Digest: "sha256:8f14e45fceea167a5a36dedd4bea2543",
			Labels: map[string]string{"channel": "stable"},
		}},
		Steps: []deploy.StepResult{{
			StepID:  "deploy",
			Status:  deploy.StatusSucceeded,
			Outputs: map[string]string{"revision": "abc123"},
		}},
		Labels:   map[string]string{"team": "platform"},
		Metadata: map[string]string{"worker": "cluster-a"},
	}
	if err := recorder.Record(context.Background(), bundle); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	bundle.Labels["team"] = "changed"
	bundle.Artifacts[0].Labels["channel"] = "changed"
	bundle.Steps[0].Outputs["revision"] = "changed"

	recorded := recorder.Bundles()
	recorded[0].Labels["team"] = "mutated-again"
	again := recorder.Bundles()

	if got := again[0].Labels["team"]; got != "platform" {
		t.Fatalf("recorded team = %q, want platform", got)
	}
	if got := again[0].Artifacts[0].Labels["channel"]; got != "stable" {
		t.Fatalf("artifact label = %q, want stable", got)
	}
	if got := again[0].Steps[0].Outputs["revision"]; got != "abc123" {
		t.Fatalf("step output = %q, want abc123", got)
	}
}

func TestMemoryRecorderHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewMemoryRecorder().Record(ctx, Bundle{ID: "bundle-1"})
	if err == nil {
		t.Fatal("Record() error = nil, want context cancellation")
	}
}
