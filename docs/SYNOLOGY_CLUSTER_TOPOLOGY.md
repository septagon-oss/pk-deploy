# Synology Control Plane + Cluster Worker

This is the first deployment shape `pk-deploy` optimizes for.

## Runtime Layout

```text
Synology
  pk-deploy control plane
  PostgreSQL or SQLite job store
  evidence store
  Prometheus
  Grafana

Kubernetes cluster
  pk-deploy worker
  scoped service account
  Flux / Helm / kubectl executors
  /metrics endpoint
```

## Connectivity

Prefer outbound worker polling:

```text
worker -> control plane: claim job
worker -> control plane: ack/nack job
worker -> control plane: upload evidence
Prometheus -> worker: scrape /metrics
```

If Synology cannot reach the cluster, run a Prometheus agent or OpenTelemetry
Collector in the cluster and remote-write to the Synology observability stack.

## First PlatformKit Profile

The PlatformKit profile should register executors for:

- `flux.reconcile-source`
- `flux.reconcile-kustomization`
- `flux.reconcile-helmrelease`
- `kubernetes.rollout-status`
- `http.smoke-check`

Those are profile executors, not core concepts. The same core can later register
Docker Compose, SSH, Terraform, Pulumi, ECS, Nomad, Fly.io, Render, or Vercel
executors.

## Seed Runtime

The seed runtime ships two binaries in one image:

- `pk-deploy-controlplane`
- `pk-deploy-worker`

The control plane runs on Synology through
`deploy/synology/docker-compose.yaml`. The worker runs in Kubernetes through
`deploy/kubernetes/worker.yaml` and exposes only the concrete `http.get`,
`kubernetes.inventory`, and `kubernetes.set-image` executors. Additional
deployment engines belong in separate adapter packages.

## Minimum Dashboard

The worker metrics are intentionally simple:

- `pk_deploy_jobs_started_total`
- `pk_deploy_jobs_completed_total{status="succeeded|failed"}`
- `pk_deploy_active_jobs`
- `pk_deploy_job_duration_seconds_count`
- `pk_deploy_job_duration_seconds_sum`

Grafana should show:

- worker heartbeat age
- deployments started per environment
- success/failure ratio
- current active deployments
- p95 duration once a histogram adapter is added
- latest failed step and evidence link
