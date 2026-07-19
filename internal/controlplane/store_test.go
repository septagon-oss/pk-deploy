// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package controlplane

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

func TestStoreReplacesKubernetesInventorySnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 20, 9, 30, 0, 0, time.UTC)
	store := NewStore(now)
	completeInventory(t, store, now, []deploy.ComponentState{
		component("platformkit-staging/old-service/old", "old-service"),
		component("platformkit-staging/gateway/gateway", "gateway"),
	})
	completeInventory(t, store, now.Add(time.Minute), []deploy.ComponentState{
		component("platformkit-staging/gateway/gateway", "gateway"),
	})

	snapshot := store.Snapshot()
	if len(snapshot.Components) != 1 {
		t.Fatalf("component count = %d, want 1: %#v", len(snapshot.Components), snapshot.Components)
	}
	if got := snapshot.Components[0].ID; got != "platformkit-staging/gateway/gateway" {
		t.Fatalf("component id = %q", got)
	}
}

func TestStoreKeepsNonKubernetesComponentWhenReplacingKubernetesInventory(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 20, 9, 30, 0, 0, time.UTC)
	store := NewStore(now)
	if err := store.UpsertComponents([]deploy.ComponentState{{
		ID:            "staging/external-control-plane/api",
		Name:          "external-control-plane",
		EnvironmentID: "staging",
		Runtime:       "docker",
		Status:        deploy.StatusSucceeded,
	}}); err != nil {
		t.Fatalf("upsert external component: %v", err)
	}
	completeInventory(t, store, now, []deploy.ComponentState{
		component("platformkit-staging/gateway/gateway", "gateway"),
	})

	snapshot := store.Snapshot()
	if len(snapshot.Components) != 2 {
		t.Fatalf("component count = %d, want 2: %#v", len(snapshot.Components), snapshot.Components)
	}
	if _, ok := store.Component("staging/external-control-plane/api"); !ok {
		t.Fatal("non-kubernetes component was removed")
	}
}

func completeInventory(t *testing.T, store *Store, observedAt time.Time, components []deploy.ComponentState) {
	t.Helper()
	plan := inventoryPlan(observedAt)
	signed, err := job.Sign(plan, "test", []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("sign inventory job: %v", err)
	}
	if err := store.Enqueue(signed); err != nil {
		t.Fatalf("enqueue inventory job: %v", err)
	}
	claimed, err := store.Claim(t.Context(), worker.Info{
		Capabilities: []string{kubernetesInventoryExecutor},
		Labels:       map[string]string{"environment": "staging"},
	})
	if err != nil {
		t.Fatalf("claim inventory job: %v", err)
	}
	if claimed.Job.ID != plan.ID {
		t.Fatalf("claimed job = %q, want %q", claimed.Job.ID, plan.ID)
	}
	raw, err := json.Marshal(components)
	if err != nil {
		t.Fatalf("marshal components: %v", err)
	}
	if err := store.Complete(t.Context(), worker.Result{
		JobID:      plan.ID,
		PlanID:     plan.Plan.ID,
		Status:     deploy.StatusSucceeded,
		StartedAt:  observedAt,
		FinishedAt: observedAt.Add(time.Second),
		Steps: []deploy.StepResult{{
			StepID:     "kubernetes-inventory",
			Status:     deploy.StatusSucceeded,
			StartedAt:  observedAt,
			FinishedAt: observedAt.Add(time.Second),
			Outputs: map[string]string{
				"components": string(raw),
			},
		}},
	}); err != nil {
		t.Fatalf("complete inventory: %v", err)
	}
}

func inventoryPlan(now time.Time) job.Job {
	return job.Job{
		ID: "inventory-" + now.Format("20060102T150405Z"),
		Plan: deploy.Plan{
			ID: "inventory-plan-" + now.Format("20060102T150405Z"),
			Application: deploy.Application{
				ID:   inventoryApplicationID,
				Name: "Deployment inventory",
			},
			Environment: deploy.Environment{ID: "staging", Name: "staging"},
			Steps: []deploy.Step{{
				ID:       "kubernetes-inventory",
				Executor: kubernetesInventoryExecutor,
				Action:   "list-deployments",
			}},
			CreatedAt: now,
		},
		Selector:  deploy.WorkerSelector{Capabilities: []string{kubernetesInventoryExecutor}, Labels: map[string]string{"environment": "staging"}},
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
		Nonce:     "nonce-" + now.Format("20060102T150405.000000000Z"),
	}
}

func component(id string, name string) deploy.ComponentState {
	return deploy.ComponentState{
		ID:              id,
		Name:            name,
		EnvironmentID:   "staging",
		Namespace:       "platformkit-staging",
		WorkloadKind:    "Deployment",
		WorkloadName:    name,
		Container:       name,
		Runtime:         kubernetesRuntime,
		CurrentImage:    "registry.example/" + name + ":1.0.0",
		CurrentVersion:  "1.0.0",
		ReadyReplicas:   1,
		DesiredReplicas: 1,
		Status:          deploy.StatusSucceeded,
	}
}
