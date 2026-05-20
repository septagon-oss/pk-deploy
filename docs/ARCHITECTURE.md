# Architecture

`pk-deploy` models deployment as signed intent plus runtime-local execution.

The control plane answers: what should happen?

The worker answers: can this runtime safely do it, and what happened?

## Core Blocks

| Block | Responsibility | Extension Point |
| --- | --- | --- |
| Plan | Declares application, environment, artifacts, gates, checks, rollback, and ordered steps. | Additional artifact/check/gate kinds. |
| Job | Wraps a plan with issuance, expiry, nonce, and worker selector. | Alternative signing implementations. |
| Worker | Claims signed jobs, verifies intent, executes steps, and reports completion. | Sources, executors, evidence sinks, metrics sinks. |
| Executor | Performs one concrete action such as Flux reconcile, Helm upgrade, Docker Compose up, or SSH command. | Adapter packages register executors by name. |
| Evidence | Records plan, job, step, check, and artifact outcomes in one immutable bundle. | Durable stores, object storage, audit logs. |
| Metrics | Emits deployment health as Prometheus text without forcing a metrics SDK. | Prometheus, OpenTelemetry, Datadog, or custom bridges. |

## Security Model

The safe default is pull-based execution:

1. The control plane creates a deployment job from a validated plan.
2. The control plane signs the job.
3. The worker pulls the job over outbound connectivity.
4. The worker verifies the signature, expiry, and selector.
5. The worker executes only registered executors.
6. The worker reports evidence and metrics.

Runtime credentials stay with the worker. The control plane does not need a
cluster-admin kubeconfig.

## Composition Laws

`pk-deploy` blocks are release-grade only when they obey these laws:

- **Identity**: every plan, job, step, gate, and artifact has a stable ID.
- **Closure**: executing a valid job produces a complete result or a typed
  failure; no half-state escapes the worker contract.
- **Determinism**: validation, step ordering, evidence ordering, and metrics
  names are deterministic.
- **Substitution**: executors and stores can be replaced through interfaces
  without changing plans.
- **Least privilege**: the worker owns runtime credentials; the control plane
  owns signed intent.
- **Evidence**: every execution path records enough facts to audit or rollback.

## Adapter Boundary

Adapter packages should be thin and replaceable. A Flux executor should know
Flux. A Helm executor should know Helm. The core should never import Kubernetes,
GitHub, Terraform, Pulumi, Docker, SSH, or cloud SDK packages directly.

That keeps the OSS kernel reusable for PlatformKit, home-lab deployments,
enterprise Kubernetes, VM fleets, and non-Kubernetes products.
