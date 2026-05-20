package worker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

// ExecuteRequest is passed to an executor for one plan step.
type ExecuteRequest struct {
	JobID  string
	Plan   deploy.Plan
	Step   deploy.Step
	Worker Info
}

// Executor performs one adapter-defined deployment action.
type Executor interface {
	Execute(context.Context, ExecuteRequest) (deploy.StepResult, error)
}

// ExecutorFunc adapts a function to Executor.
type ExecutorFunc func(context.Context, ExecuteRequest) (deploy.StepResult, error)

// Execute implements Executor.
func (fn ExecutorFunc) Execute(ctx context.Context, req ExecuteRequest) (deploy.StepResult, error) {
	return fn(ctx, req)
}

// Registry is a concurrency-safe executor registry.
type Registry struct {
	mu        sync.RWMutex
	executors map[string]Executor
}

// NewRegistry returns an empty executor registry.
func NewRegistry() *Registry {
	return &Registry{executors: map[string]Executor{}}
}

// Register adds an executor by stable name.
func (r *Registry) Register(name string, executor Executor) error {
	if name == "" {
		return errors.New("executor name is required")
	}
	if executor == nil {
		return errors.New("executor is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.executors == nil {
		r.executors = map[string]Executor{}
	}
	if _, exists := r.executors[name]; exists {
		return fmt.Errorf("executor %q is already registered", name)
	}
	r.executors[name] = executor
	return nil
}

// Get returns an executor by name.
func (r *Registry) Get(name string) (Executor, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[name]
	return executor, ok
}

// Names returns registered executor names in deterministic order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.executors))
	for name := range r.executors {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
