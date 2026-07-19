// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/evidence"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/metrics"
)

// Runner executes at most one claimed job per RunOnce call.
type Runner struct {
	Info      Info
	Source    Source
	Verifier  Verifier
	Executors *Registry
	Evidence  evidence.Recorder
	Metrics   *metrics.Collector
	Clock     func() time.Time
}

// RunOnce claims, verifies, executes, records, and completes one job.
func (r Runner) RunOnce(ctx context.Context) (Result, error) {
	if err := r.validate(); err != nil {
		return Result{}, err
	}
	now := r.now()
	signed, err := r.Source.Claim(ctx, r.Info.Clone())
	if err != nil {
		return Result{}, err
	}
	r.Metrics.JobStarted()
	startedAt := now
	result := Result{
		JobID:     claimedJobID(signed),
		Status:    deploy.StatusRunning,
		StartedAt: startedAt,
	}
	defer func() {
		if result.FinishedAt.IsZero() {
			result.FinishedAt = r.now()
		}
		r.Metrics.JobFinished(result.Status, result.FinishedAt.Sub(result.StartedAt))
	}()

	verified, verifyErr := r.Verifier.Verify(ctx, signed)
	if verifyErr != nil {
		result.Status = deploy.StatusDenied
		result.Message = verifyErr.Error()
		result.FinishedAt = r.now()
		if recordErr := r.record(ctx, signed.Job, result, verifyErr); recordErr != nil {
			return result, recordErr
		}
		return result, r.Source.Complete(ctx, result.Clone())
	}
	result.JobID = verified.ID
	result.PlanID = verified.Plan.ID
	if !r.Info.Matches(verified.Selector) {
		err := errors.New("worker does not match job selector")
		result.Status = deploy.StatusDenied
		result.Message = err.Error()
		result.FinishedAt = r.now()
		if recordErr := r.record(ctx, verified, result, err); recordErr != nil {
			return result, recordErr
		}
		return result, r.Source.Complete(ctx, result.Clone())
	}

	result = r.execute(ctx, verified, result)
	if recordErr := r.record(ctx, verified, result, nil); recordErr != nil {
		return result, recordErr
	}
	return result, r.Source.Complete(ctx, result.Clone())
}

func (r Runner) execute(ctx context.Context, job job.Job, result Result) Result {
	for _, step := range job.Plan.Steps {
		stepResult := r.executeStep(ctx, job, step)
		result.Steps = append(result.Steps, stepResult)
		if stepResult.Status != deploy.StatusSucceeded {
			result.Status = stepResult.Status
			result.Message = stepResult.Message
			result.FinishedAt = stepResult.FinishedAt
			return result
		}
	}
	result.Status = deploy.StatusSucceeded
	result.FinishedAt = r.now()
	return result
}

func (r Runner) executeStep(ctx context.Context, job job.Job, step deploy.Step) deploy.StepResult {
	startedAt := r.now()
	executor, ok := r.Executors.Get(step.Executor)
	if !ok {
		return deploy.StepResult{
			StepID:     step.ID,
			Status:     deploy.StatusFailed,
			Message:    fmt.Sprintf("executor %q is not registered", step.Executor),
			StartedAt:  startedAt,
			FinishedAt: r.now(),
		}
	}
	stepCtx := ctx
	var cancel context.CancelFunc
	if step.TimeoutSeconds > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	stepResult, err := executor.Execute(stepCtx, ExecuteRequest{
		JobID:  job.ID,
		Plan:   job.Plan.Clone(),
		Step:   step.Clone(),
		Worker: r.Info.Clone(),
	})
	if stepResult.StepID == "" {
		stepResult.StepID = step.ID
	}
	if stepResult.StartedAt.IsZero() {
		stepResult.StartedAt = startedAt
	}
	if stepResult.FinishedAt.IsZero() {
		stepResult.FinishedAt = r.now()
	}
	if err != nil {
		stepResult.Status = deploy.StatusFailed
		if stepResult.Message == "" {
			stepResult.Message = err.Error()
		}
	}
	if stepResult.Status == "" {
		stepResult.Status = deploy.StatusSucceeded
	}
	return stepResult.Clone()
}

func (r Runner) record(ctx context.Context, job job.Job, result Result, err error) error {
	if r.Evidence == nil {
		return nil
	}
	bundle := evidence.Bundle{
		ID:            result.JobID,
		JobID:         result.JobID,
		PlanID:        result.PlanID,
		ApplicationID: job.Plan.Application.ID,
		EnvironmentID: job.Plan.Environment.ID,
		Status:        result.Status,
		StartedAt:     result.StartedAt,
		FinishedAt:    result.FinishedAt,
		Artifacts:     job.Plan.Artifacts,
		Steps:         result.Steps,
		Labels:        job.Plan.Labels,
	}
	if err != nil {
		bundle.Errors = []string{err.Error()}
	}
	return r.Evidence.Record(ctx, bundle)
}

func (r Runner) validate() error {
	var errs []error
	if r.Info.ID == "" {
		errs = append(errs, errors.New("worker info id is required"))
	}
	if r.Source == nil {
		errs = append(errs, errors.New("worker source is required"))
	}
	if r.Verifier == nil {
		errs = append(errs, errors.New("worker verifier is required"))
	}
	if r.Executors == nil {
		errs = append(errs, errors.New("worker executor registry is required"))
	}
	if r.Metrics == nil {
		errs = append(errs, errors.New("worker metrics collector is required"))
	}
	return errors.Join(errs...)
}

func (r Runner) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now().UTC()
}

func claimedJobID(signed job.SignedJob) string {
	if signed.Job.ID != "" {
		return signed.Job.ID
	}
	if len(signed.Signature) >= 12 {
		return "unverified-" + signed.Signature[:12]
	}
	return "unverified"
}
