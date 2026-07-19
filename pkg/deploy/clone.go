// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package deploy

import "maps"

// Clone returns a deep copy of the plan's mutable slices and maps.
func (p Plan) Clone() Plan {
	out := p
	out.Application = p.Application.Clone()
	out.Environment = p.Environment.Clone()
	out.Artifacts = cloneSliceFunc(p.Artifacts, Artifact.Clone)
	out.Gates = cloneSliceFunc(p.Gates, Gate.Clone)
	out.Checks = cloneSliceFunc(p.Checks, Check.Clone)
	out.Steps = cloneSliceFunc(p.Steps, Step.Clone)
	out.Labels = maps.Clone(p.Labels)
	if p.Rollback != nil {
		rollback := p.Rollback.Clone()
		out.Rollback = &rollback
	}
	return out
}

// Clone returns a deep copy of the application.
func (a Application) Clone() Application {
	a.Components = cloneSliceFunc(a.Components, Component.Clone)
	a.Labels = maps.Clone(a.Labels)
	return a
}

// Clone returns a deep copy of the component.
func (c Component) Clone() Component {
	c.Labels = maps.Clone(c.Labels)
	return c
}

// Clone returns a deep copy of the component state.
func (c ComponentState) Clone() ComponentState {
	c.Labels = maps.Clone(c.Labels)
	return c
}

// Clone returns a deep copy of the environment.
func (e Environment) Clone() Environment {
	e.Targets = cloneSliceFunc(e.Targets, Target.Clone)
	e.Labels = maps.Clone(e.Labels)
	return e
}

// Clone returns a deep copy of the target.
func (t Target) Clone() Target {
	t.Labels = maps.Clone(t.Labels)
	return t
}

// Clone returns a deep copy of the artifact.
func (a Artifact) Clone() Artifact {
	a.Labels = maps.Clone(a.Labels)
	return a
}

// Clone returns a deep copy of the gate.
func (g Gate) Clone() Gate {
	g.Labels = maps.Clone(g.Labels)
	return g
}

// Clone returns a deep copy of the check.
func (c Check) Clone() Check {
	c.Inputs = maps.Clone(c.Inputs)
	return c
}

// Clone returns a deep copy of the step.
func (s Step) Clone() Step {
	s.Inputs = maps.Clone(s.Inputs)
	return s
}

// Clone returns a deep copy of the rollback target.
func (r RollbackTarget) Clone() RollbackTarget {
	r.Artifacts = cloneSliceFunc(r.Artifacts, Artifact.Clone)
	r.Labels = maps.Clone(r.Labels)
	return r
}

// Clone returns a deep copy of the worker selector.
func (s WorkerSelector) Clone() WorkerSelector {
	s.Capabilities = append([]string(nil), s.Capabilities...)
	s.Labels = maps.Clone(s.Labels)
	return s
}

// Clone returns a deep copy of the step result.
func (r StepResult) Clone() StepResult {
	r.Outputs = maps.Clone(r.Outputs)
	return r
}

// Clone returns a deep copy of the check result.
func (r CheckResult) Clone() CheckResult {
	r.Outputs = maps.Clone(r.Outputs)
	return r
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
