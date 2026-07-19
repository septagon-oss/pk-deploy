// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/evidence"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/metrics"
)

func TestRunnerRunOnceExecutesSignedJobAndRecordsEvidence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := []byte(strings.Repeat("s", 32))
	signed := signJob(t, now, secret, deploy.WorkerSelector{
		Capabilities: []string{"flux"},
		Labels:       map[string]string{"cluster": "staging"},
	})
	source := &memorySource{next: signed}
	registry := NewRegistry()
	if err := registry.Register("flux", ExecutorFunc(func(_ context.Context, req ExecuteRequest) (deploy.StepResult, error) {
		if req.Step.Action != "reconcile" {
			t.Fatalf("unexpected action %q", req.Step.Action)
		}
		return deploy.StepResult{Outputs: map[string]string{"revision": "abc123"}}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	recorder := evidence.NewMemoryRecorder()
	var collector metrics.Collector

	result, err := Runner{
		Info:      Info{ID: "worker-1", Capabilities: []string{"flux"}, Labels: map[string]string{"cluster": "staging"}},
		Source:    source,
		Verifier:  verifier(secret, now.Add(time.Minute)),
		Executors: registry,
		Evidence:  recorder,
		Metrics:   &collector,
		Clock:     fixedClock(now),
	}.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Status != deploy.StatusSucceeded {
		t.Fatalf("result status = %q", result.Status)
	}
	if len(result.Steps) != 1 || result.Steps[0].Outputs["revision"] != "abc123" {
		t.Fatalf("unexpected step results: %#v", result.Steps)
	}
	if source.completed.Status != deploy.StatusSucceeded {
		t.Fatalf("completed status = %q", source.completed.Status)
	}
	if len(source.completed.Steps) != 1 || source.completed.Steps[0].StepID != "deploy" || source.completed.Steps[0].Status != deploy.StatusSucceeded {
		t.Fatalf("completed step was not preserved: %#v", source.completed.Steps)
	}
	bundles := recorder.Bundles()
	if len(bundles) != 1 || bundles[0].Status != deploy.StatusSucceeded || bundles[0].ApplicationID != "app" {
		t.Fatalf("unexpected evidence: %#v", bundles)
	}
	if snapshot := collector.Snapshot(); snapshot.Started != 1 || snapshot.Succeeded != 1 || snapshot.Active != 0 {
		t.Fatalf("unexpected metrics snapshot: %#v", snapshot)
	}
}

func TestRunnerRunOnceDeniesSelectorMismatchBeforeExecution(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := []byte(strings.Repeat("s", 32))
	signed := signJob(t, now, secret, deploy.WorkerSelector{Capabilities: []string{"helm"}})
	source := &memorySource{next: signed}
	registry := NewRegistry()
	executed := false
	if err := registry.Register("flux", ExecutorFunc(func(context.Context, ExecuteRequest) (deploy.StepResult, error) {
		executed = true
		return deploy.StepResult{}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	result, err := Runner{
		Info:      Info{ID: "worker-1", Capabilities: []string{"flux"}},
		Source:    source,
		Verifier:  verifier(secret, now.Add(time.Minute)),
		Executors: registry,
		Metrics:   &metrics.Collector{},
		Clock:     fixedClock(now),
	}.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Status != deploy.StatusDenied {
		t.Fatalf("result status = %q, want denied", result.Status)
	}
	if executed {
		t.Fatal("executor ran despite selector mismatch")
	}
}

func TestRunnerRunOnceFailsOnMissingExecutor(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := []byte(strings.Repeat("s", 32))
	source := &memorySource{next: signJob(t, now, secret, deploy.WorkerSelector{})}

	result, err := Runner{
		Info:      Info{ID: "worker-1"},
		Source:    source,
		Verifier:  verifier(secret, now.Add(time.Minute)),
		Executors: NewRegistry(),
		Metrics:   &metrics.Collector{},
		Clock:     fixedClock(now),
	}.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Status != deploy.StatusFailed || !strings.Contains(result.Message, "not registered") {
		t.Fatalf("result = %#v, want missing executor failure", result)
	}
}

func TestRunnerRunOnceRejectsNilMetricsCollectorBeforeClaim(t *testing.T) {
	t.Parallel()

	source := &memorySource{}
	_, err := Runner{
		Info:      Info{ID: "worker-1"},
		Source:    source,
		Verifier:  VerifierFunc(func(context.Context, job.SignedJob) (job.Job, error) { return job.Job{}, nil }),
		Executors: NewRegistry(),
	}.RunOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker metrics collector is required") {
		t.Fatalf("RunOnce() error = %v, want missing metrics collector error", err)
	}
	if source.claimed {
		t.Fatal("source was claimed before runner configuration validation")
	}
}

func TestRegistryRejectsDuplicateExecutors(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	executor := ExecutorFunc(func(context.Context, ExecuteRequest) (deploy.StepResult, error) {
		return deploy.StepResult{}, nil
	})
	if err := registry.Register("flux", executor); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register("flux", executor); err == nil {
		t.Fatal("Register() error = nil, want duplicate error")
	}
}

type memorySource struct {
	next      job.SignedJob
	claimed   bool
	completed Result
}

func (s *memorySource) Claim(context.Context, Info) (job.SignedJob, error) {
	if s.claimed {
		return job.SignedJob{}, ErrNoJob
	}
	s.claimed = true
	return s.next.Clone(), nil
}

func (s *memorySource) Complete(_ context.Context, result Result) error {
	s.completed = result
	return nil
}

func verifier(secret []byte, now time.Time) Verifier {
	return VerifierFunc(func(_ context.Context, signed job.SignedJob) (job.Job, error) {
		return job.Verify(signed, job.KeyResolverFunc(func(keyID string) ([]byte, error) {
			if keyID != "local" {
				return nil, errors.New("unknown key")
			}
			return secret, nil
		}), now)
	})
}

func signJob(t *testing.T, now time.Time, secret []byte, selector deploy.WorkerSelector) job.SignedJob {
	t.Helper()
	signed, err := job.Sign(job.Job{
		ID:        "job-1",
		Plan:      validPlan(now),
		Selector:  selector,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
		Nonce:     "nonce-1",
	}, "local", secret)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return signed
}

func validPlan(now time.Time) deploy.Plan {
	return deploy.Plan{
		ID: "release-1",
		Application: deploy.Application{
			ID: "app",
		},
		Environment: deploy.Environment{
			ID: "staging",
		},
		Artifacts: []deploy.Artifact{{
			ID:     "image",
			Kind:   "oci-image",
			Ref:    "registry.example.com/app:v1",
			Digest: "sha256:8f14e45fceea167a5a36dedd4bea2543",
		}},
		Gates: []deploy.Gate{{
			ID:       "approval",
			Kind:     "manual",
			Required: true,
			Status:   deploy.StatusSucceeded,
		}},
		Steps: []deploy.Step{{
			ID:       "deploy",
			Executor: "flux",
			Action:   "reconcile",
		}},
		CreatedAt: now,
	}
}

func fixedClock(now time.Time) func() time.Time {
	return func() time.Time { return now }
}
