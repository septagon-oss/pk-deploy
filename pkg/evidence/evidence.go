// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

// Package evidence records immutable deployment execution facts.
package evidence

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

// Bundle is the audit record for one job execution.
type Bundle struct {
	ID            string               `json:"id"`
	JobID         string               `json:"jobId"`
	PlanID        string               `json:"planId"`
	ApplicationID string               `json:"applicationId"`
	EnvironmentID string               `json:"environmentId"`
	Status        deploy.Status        `json:"status"`
	StartedAt     time.Time            `json:"startedAt"`
	FinishedAt    time.Time            `json:"finishedAt"`
	Artifacts     []deploy.Artifact    `json:"artifacts,omitempty"`
	Steps         []deploy.StepResult  `json:"steps,omitempty"`
	Checks        []deploy.CheckResult `json:"checks,omitempty"`
	Labels        map[string]string    `json:"labels,omitempty"`
	Metadata      map[string]string    `json:"metadata,omitempty"`
	Errors        []string             `json:"errors,omitempty"`
}

// Recorder persists execution evidence.
type Recorder interface {
	Record(context.Context, Bundle) error
}

// MemoryRecorder is a concurrency-safe recorder for tests, local development,
// and single-node control-plane prototypes.
type MemoryRecorder struct {
	mu      sync.Mutex
	bundles []Bundle
}

// NewMemoryRecorder returns an empty recorder.
func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{}
}

// Record stores a defensive copy of a bundle.
func (r *MemoryRecorder) Record(ctx context.Context, bundle Bundle) error {
	if r == nil {
		return errors.New("evidence recorder is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if bundle.ID == "" {
		return errors.New("evidence bundle id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bundles = append(r.bundles, bundle.Clone())
	return nil
}

// Bundles returns defensive copies of recorded bundles.
func (r *MemoryRecorder) Bundles() []Bundle {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneBundles(r.bundles)
}

// Clone returns a deep copy of the bundle.
func (b Bundle) Clone() Bundle {
	b.Artifacts = cloneSliceFunc(b.Artifacts, deploy.Artifact.Clone)
	b.Steps = cloneSliceFunc(b.Steps, deploy.StepResult.Clone)
	b.Checks = cloneSliceFunc(b.Checks, deploy.CheckResult.Clone)
	b.Labels = maps.Clone(b.Labels)
	b.Metadata = maps.Clone(b.Metadata)
	b.Errors = slices.Clone(b.Errors)
	return b
}

func cloneBundles(values []Bundle) []Bundle {
	return cloneSliceFunc(values, Bundle.Clone)
}

func cloneSliceFunc[T any](values []T, clone func(T) T) []T {
	if values == nil {
		return nil
	}
	out := make([]T, len(values))
	for i, value := range values {
		out[i] = clone(value)
	}
	return out
}
