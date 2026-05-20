package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/evidence"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

const (
	inventoryApplicationID      = "pk-deploy-inventory"
	kubernetesInventoryExecutor = "kubernetes.inventory"
	kubernetesRuntime           = "kubernetes"
)

// Store is an in-memory control-plane store for the first self-hosted runtime.
type Store struct {
	mu         sync.Mutex
	pending    []job.SignedJob
	claimed    map[string]job.Job
	completed  []worker.Result
	evidence   []evidence.Bundle
	components map[string]deploy.ComponentState
	startedAt  time.Time
}

// NewStore returns an empty in-memory store.
func NewStore(now time.Time) *Store {
	return &Store{
		claimed:    make(map[string]job.Job),
		components: make(map[string]deploy.ComponentState),
		startedAt:  now,
	}
}

// Enqueue appends a signed job to the pending queue.
func (s *Store) Enqueue(signed job.SignedJob) error {
	if s == nil {
		return errors.New("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, signed.Clone())
	return nil
}

// Claim returns the first pending job matching the worker selector.
func (s *Store) Claim(_ context.Context, info worker.Info) (job.SignedJob, error) {
	if s == nil {
		return job.SignedJob{}, errors.New("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, signed := range s.pending {
		if !info.Matches(signed.Job.Selector) {
			continue
		}
		s.pending = slices.Delete(s.pending, i, i+1)
		s.claimed[signed.Job.ID] = signed.Job.Clone()
		return signed.Clone(), nil
	}
	return job.SignedJob{}, worker.ErrNoJob
}

// Complete records a terminal worker result and the corresponding evidence.
func (s *Store) Complete(_ context.Context, result worker.Result) error {
	if s == nil {
		return errors.New("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	completed := result.Clone()
	s.completed = append(s.completed, completed)
	if claimed, ok := s.claimed[completed.JobID]; ok {
		delete(s.claimed, completed.JobID)
		s.evidence = append(s.evidence, evidenceFromClaimedJob(claimed, completed))
		s.ingestComponentState(claimed, completed)
		return nil
	}
	s.evidence = append(s.evidence, evidenceFromResult(completed))
	s.ingestComponentState(job.Job{}, completed)
	return nil
}

// Record stores an evidence bundle.
func (s *Store) Record(_ context.Context, bundle evidence.Bundle) error {
	if s == nil {
		return errors.New("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence = append(s.evidence, bundle.Clone())
	return nil
}

// Snapshot returns a defensive copy of current control-plane state.
func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := make([]JobSummary, 0, len(s.pending))
	for _, signed := range s.pending {
		pending = append(pending, JobSummary{
			ID:            signed.Job.ID,
			PlanID:        signed.Job.Plan.ID,
			ApplicationID: signed.Job.Plan.Application.ID,
			EnvironmentID: signed.Job.Plan.Environment.ID,
			ExecutorCount: len(signed.Job.Plan.Steps),
			ExpiresAt:     signed.Job.ExpiresAt,
		})
	}
	completed := make([]worker.Result, len(s.completed))
	for i, result := range s.completed {
		completed[i] = result.Clone()
	}
	bundles := make([]evidence.Bundle, len(s.evidence))
	for i, bundle := range s.evidence {
		bundles[i] = bundle.Clone()
	}
	components := make([]deploy.ComponentState, 0, len(s.components))
	readyComponents := 0
	staleComponents := 0
	for _, component := range s.components {
		if component.Status == deploy.StatusSucceeded {
			readyComponents++
		}
		if component.UpdateAvailable {
			staleComponents++
		}
		components = append(components, component.Clone())
	}
	slices.SortFunc(components, func(a, b deploy.ComponentState) int {
		if a.EnvironmentID != b.EnvironmentID {
			return cmpString(a.EnvironmentID, b.EnvironmentID)
		}
		return cmpString(a.ID, b.ID)
	})
	return Snapshot{
		StartedAt:       s.startedAt,
		Components:      components,
		ReadyComponents: readyComponents,
		StaleComponents: staleComponents,
		Pending:         pending,
		Completed:       completed,
		Evidence:        bundles,
	}
}

// Component returns one observed component by id.
func (s *Store) Component(id string) (deploy.ComponentState, bool) {
	if s == nil {
		return deploy.ComponentState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	component, ok := s.components[id]
	return component.Clone(), ok
}

// UpsertComponents stores component state observations.
func (s *Store) UpsertComponents(components []deploy.ComponentState) error {
	if s == nil {
		return errors.New("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertComponentsLocked(components)
	return nil
}

// Snapshot is rendered by the status API and web UI.
type Snapshot struct {
	StartedAt       time.Time               `json:"startedAt"`
	Components      []deploy.ComponentState `json:"components"`
	ReadyComponents int                     `json:"readyComponents"`
	StaleComponents int                     `json:"staleComponents"`
	Pending         []JobSummary            `json:"pendingJobs"`
	Completed       []worker.Result         `json:"completedJobs"`
	Evidence        []evidence.Bundle       `json:"evidence"`
}

// JobSummary is a safe summary of a pending signed job.
type JobSummary struct {
	ID            string    `json:"id"`
	PlanID        string    `json:"planId"`
	ApplicationID string    `json:"applicationId"`
	EnvironmentID string    `json:"environmentId"`
	ExecutorCount int       `json:"executorCount"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

// Metrics renders control-plane Prometheus metrics.
func (s *Store) Metrics() string {
	snapshot := s.Snapshot()
	succeeded := 0
	failed := 0
	for _, result := range snapshot.Completed {
		if result.Status == deploy.StatusSucceeded {
			succeeded++
		} else {
			failed++
		}
	}
	return "# HELP pk_deploy_control_pending_jobs Pending signed deployment jobs.\n" +
		"# TYPE pk_deploy_control_pending_jobs gauge\n" +
		formatMetric("pk_deploy_control_pending_jobs", len(snapshot.Pending)) +
		"# HELP pk_deploy_control_components Observed deployable components.\n" +
		"# TYPE pk_deploy_control_components gauge\n" +
		formatMetric("pk_deploy_control_components", len(snapshot.Components)) +
		"# HELP pk_deploy_control_stale_components Components with an observed newer image tag.\n" +
		"# TYPE pk_deploy_control_stale_components gauge\n" +
		formatMetric("pk_deploy_control_stale_components", snapshot.StaleComponents) +
		"# HELP pk_deploy_control_completed_jobs_total Completed jobs by status.\n" +
		"# TYPE pk_deploy_control_completed_jobs_total counter\n" +
		formatMetric(`pk_deploy_control_completed_jobs_total{status="succeeded"}`, succeeded) +
		formatMetric(`pk_deploy_control_completed_jobs_total{status="failed"}`, failed) +
		"# HELP pk_deploy_control_evidence_bundles_total Evidence bundles recorded by the control plane.\n" +
		"# TYPE pk_deploy_control_evidence_bundles_total counter\n" +
		formatMetric("pk_deploy_control_evidence_bundles_total", len(snapshot.Evidence))
}

func formatMetric(name string, value int) string {
	return name + " " + strconv.Itoa(value) + "\n"
}

func (s *Store) ingestComponentState(claimed job.Job, result worker.Result) {
	for _, step := range result.Steps {
		raw := step.Outputs["components"]
		if raw == "" {
			continue
		}
		var components []deploy.ComponentState
		if err := json.Unmarshal([]byte(raw), &components); err != nil {
			continue
		}
		if isKubernetesInventory(claimed, step) {
			s.replaceKubernetesInventoryLocked(claimed.Plan.Environment.ID, components)
			continue
		}
		s.upsertComponentsLocked(components)
	}
}

func isKubernetesInventory(claimed job.Job, step deploy.StepResult) bool {
	if claimed.Plan.Application.ID != inventoryApplicationID {
		return false
	}
	for _, planStep := range claimed.Plan.Steps {
		if planStep.ID == step.StepID {
			return planStep.Executor == kubernetesInventoryExecutor
		}
	}
	return false
}

func (s *Store) replaceKubernetesInventoryLocked(environmentID string, components []deploy.ComponentState) {
	for _, component := range components {
		if environmentID == "" {
			environmentID = component.EnvironmentID
			break
		}
	}
	if environmentID == "" {
		return
	}
	for id, component := range s.components {
		if component.EnvironmentID == environmentID && component.Runtime == kubernetesRuntime {
			delete(s.components, id)
		}
	}
	s.upsertComponentsLocked(components)
}

func (s *Store) upsertComponentsLocked(components []deploy.ComponentState) {
	for _, component := range components {
		if component.ID == "" {
			continue
		}
		s.components[component.ID] = component.Clone()
	}
}

func evidenceFromClaimedJob(claimed job.Job, result worker.Result) evidence.Bundle {
	bundle := evidenceFromResult(result)
	bundle.ApplicationID = claimed.Plan.Application.ID
	bundle.EnvironmentID = claimed.Plan.Environment.ID
	bundle.Artifacts = cloneSliceFunc(claimed.Plan.Artifacts, deploy.Artifact.Clone)
	bundle.Labels = maps.Clone(claimed.Plan.Labels)
	return bundle
}

func evidenceFromResult(result worker.Result) evidence.Bundle {
	return evidence.Bundle{
		ID:         result.JobID,
		JobID:      result.JobID,
		PlanID:     result.PlanID,
		Status:     result.Status,
		StartedAt:  result.StartedAt,
		FinishedAt: result.FinishedAt,
		Steps:      cloneSliceFunc(result.Steps, deploy.StepResult.Clone),
	}
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

func cmpString(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
