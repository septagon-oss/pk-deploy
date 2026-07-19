// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package deploy

import (
	"strings"
	"testing"
	"time"
)

func TestPlanValidateAcceptsMinimalReleaseGradePlan(t *testing.T) {
	t.Parallel()

	plan := validPlan()
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPlanValidateRejectsMutableArtifactRefWithoutDigest(t *testing.T) {
	t.Parallel()

	plan := validPlan()
	plan.Artifacts[0].Digest = ""
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), "plan.artifacts.digest") {
		t.Fatalf("Validate() error = %v, want digest error", err)
	}
}

func TestPlanValidateRejectsDuplicateStepIDs(t *testing.T) {
	t.Parallel()

	plan := validPlan()
	plan.Steps = append(plan.Steps, plan.Steps[0])
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), `duplicate id "deploy"`) {
		t.Fatalf("Validate() error = %v, want duplicate step error", err)
	}
}

func TestPlanValidateRejectsUnsatisfiedRequiredGate(t *testing.T) {
	t.Parallel()

	plan := validPlan()
	plan.Gates[0].Status = StatusPending
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), "required gate") {
		t.Fatalf("Validate() error = %v, want required gate error", err)
	}
}

func TestPlanCloneDefensivelyCopiesMutableFields(t *testing.T) {
	t.Parallel()

	plan := validPlan()
	clone := plan.Clone()
	clone.Labels["team"] = "changed"
	clone.Application.Components[0].Labels["tier"] = "changed"
	clone.Steps[0].Inputs["release"] = "changed"
	clone.Rollback.Artifacts[0].Labels["channel"] = "changed"

	if plan.Labels["team"] != "platform" {
		t.Fatalf("plan labels were mutated: %#v", plan.Labels)
	}
	if plan.Application.Components[0].Labels["tier"] != "api" {
		t.Fatalf("component labels were mutated: %#v", plan.Application.Components[0].Labels)
	}
	if plan.Steps[0].Inputs["release"] != "api" {
		t.Fatalf("step inputs were mutated: %#v", plan.Steps[0].Inputs)
	}
	if plan.Rollback.Artifacts[0].Labels["channel"] != "stable" {
		t.Fatalf("rollback artifact labels were mutated: %#v", plan.Rollback.Artifacts[0].Labels)
	}
}

func validPlan() Plan {
	return Plan{
		ID: "release-2026-05-19",
		Application: Application{
			ID:   "billing-api",
			Name: "Billing API",
			Components: []Component{{
				ID:      "api",
				Runtime: "kubernetes",
				Labels:  map[string]string{"tier": "api"},
			}},
		},
		Environment: Environment{
			ID:   "staging",
			Name: "Staging",
			Targets: []Target{{
				ID:   "cluster-a",
				Kind: "kubernetes",
			}},
		},
		Artifacts: []Artifact{{
			ID:     "api-image",
			Kind:   "oci-image",
			Ref:    "registry.example.com/billing-api:v1",
			Digest: "sha256:8f14e45fceea167a5a36dedd4bea2543",
			Labels: map[string]string{"channel": "stable"},
		}},
		Gates: []Gate{{
			ID:       "change-approved",
			Kind:     "manual-approval",
			Required: true,
			Status:   StatusSucceeded,
		}},
		Checks: []Check{{
			ID:       "smoke",
			Kind:     "http",
			Required: true,
		}},
		Steps: []Step{{
			ID:             "deploy",
			Executor:       "flux",
			Action:         "reconcile-helmrelease",
			Inputs:         map[string]string{"release": "api"},
			TimeoutSeconds: 180,
		}},
		Rollback: &RollbackTarget{
			ID:       "previous",
			Strategy: "reconcile-previous-artifact",
			Artifacts: []Artifact{{
				ID:     "api-image",
				Kind:   "oci-image",
				Ref:    "registry.example.com/billing-api:v0",
				Digest: "sha256:45c48cce2e2d7fbdea1afc51c7c6ad26",
				Labels: map[string]string{"channel": "stable"},
			}},
		},
		Labels:    map[string]string{"team": "platform"},
		CreatedAt: time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC),
	}
}
