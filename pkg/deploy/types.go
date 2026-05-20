// Package deploy defines the provider-neutral deployment plan contract.
package deploy

import "time"

// Status is the normalized outcome vocabulary shared by jobs, steps, checks,
// and evidence bundles.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
	StatusDenied    Status = "denied"
)

// Plan is signed by the control plane and executed by a runtime-local worker.
// It is intentionally declarative: concrete providers are selected by executor
// name, not by importing provider SDKs into the core package.
type Plan struct {
	ID          string            `json:"id"`
	Application Application       `json:"application"`
	Environment Environment       `json:"environment"`
	Artifacts   []Artifact        `json:"artifacts,omitempty"`
	Gates       []Gate            `json:"gates,omitempty"`
	Checks      []Check           `json:"checks,omitempty"`
	Steps       []Step            `json:"steps"`
	Rollback    *RollbackTarget   `json:"rollback,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// Application identifies the product being deployed.
type Application struct {
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	Components []Component       `json:"components,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// Component identifies one deployable part of an application.
type Component struct {
	ID      string            `json:"id"`
	Runtime string            `json:"runtime,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// ComponentState is the live runtime view of one deployable component.
type ComponentState struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	EnvironmentID   string            `json:"environmentId"`
	Namespace       string            `json:"namespace,omitempty"`
	WorkloadKind    string            `json:"workloadKind,omitempty"`
	WorkloadName    string            `json:"workloadName,omitempty"`
	Container       string            `json:"container,omitempty"`
	Runtime         string            `json:"runtime,omitempty"`
	CurrentImage    string            `json:"currentImage,omitempty"`
	CurrentVersion  string            `json:"currentVersion,omitempty"`
	CurrentDigest   string            `json:"currentDigest,omitempty"`
	LatestImage     string            `json:"latestImage,omitempty"`
	LatestVersion   string            `json:"latestVersion,omitempty"`
	LatestDigest    string            `json:"latestDigest,omitempty"`
	ReadyReplicas   int               `json:"readyReplicas"`
	DesiredReplicas int               `json:"desiredReplicas"`
	Status          Status            `json:"status"`
	UpdateAvailable bool              `json:"updateAvailable"`
	LastObservedAt  time.Time         `json:"lastObservedAt"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// Environment identifies the deployment destination without prescribing the
// underlying platform.
type Environment struct {
	ID      string            `json:"id"`
	Name    string            `json:"name,omitempty"`
	Targets []Target          `json:"targets,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// Target names a runtime endpoint such as a Kubernetes namespace, VM group, or
// Docker host. The kind is adapter-defined.
type Target struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Artifact is a deployable input. Digest is required so evidence can point to
// immutable content instead of mutable tags.
type Artifact struct {
	ID            string            `json:"id"`
	Kind          string            `json:"kind"`
	Ref           string            `json:"ref"`
	Digest        string            `json:"digest"`
	SBOMRef       string            `json:"sbomRef,omitempty"`
	ProvenanceRef string            `json:"provenanceRef,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// Gate declares a pre-execution decision requirement. A core worker does not
// decide business policy; it verifies that required gates are satisfied before
// executing the plan.
type Gate struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Required bool              `json:"required"`
	Status   Status            `json:"status"`
	Reason   string            `json:"reason,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// Check declares a verification step whose implementation is adapter-defined.
type Check struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Required bool              `json:"required"`
	Status   Status            `json:"status"`
	Inputs   map[string]string `json:"inputs,omitempty"`
}

// Step is one executor action. The executor name is the extension point.
type Step struct {
	ID             string            `json:"id"`
	Executor       string            `json:"executor"`
	Action         string            `json:"action"`
	Inputs         map[string]string `json:"inputs,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
}

// RollbackTarget records the known-safe target for a one-click rollback.
type RollbackTarget struct {
	ID        string            `json:"id"`
	Strategy  string            `json:"strategy"`
	Artifacts []Artifact        `json:"artifacts,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// WorkerSelector limits which workers may claim a job.
type WorkerSelector struct {
	Capabilities []string          `json:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// StepResult is the normalized result emitted by executors.
type StepResult struct {
	StepID     string            `json:"stepId"`
	Status     Status            `json:"status"`
	Message    string            `json:"message,omitempty"`
	StartedAt  time.Time         `json:"startedAt"`
	FinishedAt time.Time         `json:"finishedAt"`
	Outputs    map[string]string `json:"outputs,omitempty"`
}

// CheckResult is the normalized result emitted by check executors.
type CheckResult struct {
	CheckID    string            `json:"checkId"`
	Status     Status            `json:"status"`
	Message    string            `json:"message,omitempty"`
	StartedAt  time.Time         `json:"startedAt"`
	FinishedAt time.Time         `json:"finishedAt"`
	Outputs    map[string]string `json:"outputs,omitempty"`
}
