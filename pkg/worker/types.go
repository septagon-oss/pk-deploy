// Package worker executes signed deployment jobs with runtime-local adapters.
package worker

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/job"
)

var ErrNoJob = errors.New("no deployment job available")

// Info describes one worker instance.
type Info struct {
	ID           string            `json:"id"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// Source claims jobs from a control plane and reports terminal results.
type Source interface {
	Claim(context.Context, Info) (job.SignedJob, error)
	Complete(context.Context, Result) error
}

// Verifier authenticates signed jobs.
type Verifier interface {
	Verify(context.Context, job.SignedJob) (job.Job, error)
}

// VerifierFunc adapts a function to Verifier.
type VerifierFunc func(context.Context, job.SignedJob) (job.Job, error)

// Verify implements Verifier.
func (fn VerifierFunc) Verify(ctx context.Context, signed job.SignedJob) (job.Job, error) {
	return fn(ctx, signed)
}

// Result is the terminal worker result sent to the source.
type Result struct {
	JobID      string              `json:"jobId"`
	PlanID     string              `json:"planId,omitempty"`
	Status     deploy.Status       `json:"status"`
	Message    string              `json:"message,omitempty"`
	StartedAt  time.Time           `json:"startedAt"`
	FinishedAt time.Time           `json:"finishedAt"`
	Steps      []deploy.StepResult `json:"steps,omitempty"`
}

// Clone returns a deep copy of worker info.
func (i Info) Clone() Info {
	i.Capabilities = slices.Clone(i.Capabilities)
	i.Labels = maps.Clone(i.Labels)
	return i
}

// Clone returns a deep copy of the result.
func (r Result) Clone() Result {
	steps := r.Steps
	r.Steps = make([]deploy.StepResult, len(steps))
	for i, step := range steps {
		r.Steps[i] = step.Clone()
	}
	return r
}

// Matches reports whether this worker satisfies a selector.
func (i Info) Matches(selector deploy.WorkerSelector) bool {
	for _, required := range selector.Capabilities {
		if !slices.Contains(i.Capabilities, required) {
			return false
		}
	}
	for key, value := range selector.Labels {
		if i.Labels[key] != value {
			return false
		}
	}
	return true
}
