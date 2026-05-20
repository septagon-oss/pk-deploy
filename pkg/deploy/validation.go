package deploy

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validate rejects incomplete or ambiguous plans before they can be signed.
func (p Plan) Validate() error {
	var errs []error
	requireToken(&errs, "plan.id", p.ID)
	if p.CreatedAt.IsZero() {
		errs = append(errs, errors.New("plan.createdAt is required"))
	}
	errs = append(errs, p.Application.validate()...)
	errs = append(errs, p.Environment.validate()...)
	errs = append(errs, validateArtifacts("plan.artifacts", p.Artifacts)...)
	errs = append(errs, validateGates(p.Gates)...)
	errs = append(errs, validateChecks(p.Checks)...)
	errs = append(errs, validateSteps(p.Steps)...)
	if p.Rollback != nil {
		errs = append(errs, p.Rollback.validate()...)
	}
	return errors.Join(errs...)
}

func (a Application) validate() []error {
	var errs []error
	requireToken(&errs, "application.id", a.ID)
	errs = append(errs, validateUnique("application.components", a.Components, func(c Component) string { return c.ID })...)
	for _, component := range a.Components {
		requireToken(&errs, "application.components.id", component.ID)
	}
	return errs
}

func (e Environment) validate() []error {
	var errs []error
	requireToken(&errs, "environment.id", e.ID)
	errs = append(errs, validateUnique("environment.targets", e.Targets, func(t Target) string { return t.ID })...)
	for _, target := range e.Targets {
		requireToken(&errs, "environment.targets.id", target.ID)
		requireToken(&errs, "environment.targets.kind", target.Kind)
	}
	return errs
}

func validateArtifacts(path string, artifacts []Artifact) []error {
	errs := validateUnique(path, artifacts, func(a Artifact) string { return a.ID })
	for _, artifact := range artifacts {
		requireToken(&errs, path+".id", artifact.ID)
		requireToken(&errs, path+".kind", artifact.Kind)
		requireString(&errs, path+".ref", artifact.Ref)
		requireString(&errs, path+".digest", artifact.Digest)
		if artifact.Digest != "" && !strings.Contains(artifact.Digest, ":") {
			errs = append(errs, fmt.Errorf("%s.digest must use algorithm:value form", path))
		}
	}
	return errs
}

func validateGates(gates []Gate) []error {
	errs := validateUnique("plan.gates", gates, func(g Gate) string { return g.ID })
	for _, gate := range gates {
		requireToken(&errs, "plan.gates.id", gate.ID)
		requireToken(&errs, "plan.gates.kind", gate.Kind)
		validateStatus(&errs, "plan.gates.status", gate.Status, true)
		if gate.Required && gate.Status != StatusSucceeded {
			errs = append(errs, fmt.Errorf("required gate %q status = %q, want %q", gate.ID, gate.Status, StatusSucceeded))
		}
	}
	return errs
}

func validateChecks(checks []Check) []error {
	errs := validateUnique("plan.checks", checks, func(c Check) string { return c.ID })
	for _, check := range checks {
		requireToken(&errs, "plan.checks.id", check.ID)
		requireToken(&errs, "plan.checks.kind", check.Kind)
		validateStatus(&errs, "plan.checks.status", check.Status, false)
		if check.Required && check.Status != "" && check.Status != StatusSucceeded {
			errs = append(errs, fmt.Errorf("required check %q status = %q, want empty or %q", check.ID, check.Status, StatusSucceeded))
		}
	}
	return errs
}

func validateStatus(errs *[]error, path string, status Status, required bool) {
	if status == "" && !required {
		return
	}
	switch status {
	case StatusPending, StatusRunning, StatusSucceeded, StatusFailed, StatusSkipped, StatusDenied:
		return
	default:
		*errs = append(*errs, fmt.Errorf("%s = %q is not a known status", path, status))
	}
}

func validateSteps(steps []Step) []error {
	var errs []error
	if len(steps) == 0 {
		errs = append(errs, errors.New("plan.steps must contain at least one step"))
	}
	errs = append(errs, validateUnique("plan.steps", steps, func(s Step) string { return s.ID })...)
	for _, step := range steps {
		requireToken(&errs, "plan.steps.id", step.ID)
		requireToken(&errs, "plan.steps.executor", step.Executor)
		requireToken(&errs, "plan.steps.action", step.Action)
		if step.TimeoutSeconds < 0 {
			errs = append(errs, fmt.Errorf("step %q timeoutSeconds must be >= 0", step.ID))
		}
	}
	return errs
}

func (r RollbackTarget) validate() []error {
	var errs []error
	requireToken(&errs, "rollback.id", r.ID)
	requireToken(&errs, "rollback.strategy", r.Strategy)
	errs = append(errs, validateArtifacts("rollback.artifacts", r.Artifacts)...)
	return errs
}

func validateUnique[T any](path string, values []T, id func(T) string) []error {
	var errs []error
	seen := map[string]struct{}{}
	for _, value := range values {
		key := strings.TrimSpace(id(value))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			errs = append(errs, fmt.Errorf("%s contains duplicate id %q", path, key))
		}
		seen[key] = struct{}{}
	}
	return errs
}

func requireToken(errs *[]error, path, value string) {
	if err := validateToken(value); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", path, err))
	}
}

func requireString(errs *[]error, path, value string) {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", path))
	}
}

func validateToken(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("is required")
	}
	if len(value) > 128 {
		return fmt.Errorf("length %d exceeds 128", len(value))
	}
	if !utf8.ValidString(value) {
		return errors.New("must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("contains invalid rune %q", r)
		}
	}
	return nil
}
