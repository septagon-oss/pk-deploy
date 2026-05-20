# pk-deploy

`pk-deploy` is a small, vendor-neutral deployment control-plane kernel.

It is built for the topology PlatformKit needs first, but it is intentionally
not PlatformKit-specific:

- a home-lab or NAS-hosted control plane creates signed deployment jobs
- a narrow worker inside each runtime pulls jobs over outbound connectivity
- executors adapt those jobs to Flux, Helm, Docker Compose, SSH, Terraform,
  Pulumi, ECS, Nomad, Fly.io, Render, or any other deploy target
- workers emit evidence and Prometheus-compatible metrics for existing
  observability stacks

The OSS core owns only the hard contracts:

- deployment plan grammar
- signed pull-based jobs
- worker execution loop
- executor registry
- evidence bundles
- Prometheus text exposition

Concrete UI, auth, persistence, Git providers, deploy executors, approval
stores, policy engines, and hosted multi-tenant features are adapters or Pro
extensions. The package boundary is deliberate: operators should be able to use
the same core for PlatformKit, a personal Synology stack, or an unrelated SaaS.

## Topology

```text
Synology / home control plane
  - web UI or CLI
  - job store
  - release registry
  - evidence store
  - Prometheus + Grafana

Cluster worker
  - outbound job polling
  - scoped Kubernetes / runtime credentials
  - executor adapters
  - evidence emission
  - /metrics endpoint
```

The control plane signs intent. The worker verifies it before doing anything.
Synology does not need broad cluster credentials.

## Quickstart

```bash
make verify
make example
```

## Runtime

Build the shared runtime image:

```bash
docker build -t 192.168.1.200:3000/atlantis/pk-deploy:latest .
```

Run the Synology control plane with
[deploy/synology/docker-compose.yaml](deploy/synology/docker-compose.yaml) and
the staging worker with
[deploy/kubernetes/worker.yaml](deploy/kubernetes/worker.yaml). The control
plane exposes:

- `/` browser interface
- `/healthz`
- `/metrics`
- `/api/status`

## Public Packages

- `pkg/deploy`: plans, artifacts, gates, checks, steps, worker selectors, and
  validation
- `pkg/job`: signed job envelopes and HMAC-SHA256 verification
- `pkg/worker`: pull-based worker loop and executor registry
- `pkg/evidence`: immutable execution evidence bundles and in-memory recorder
- `pkg/metrics`: dependency-free Prometheus text exposition

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and
[docs/SYNOLOGY_CLUSTER_TOPOLOGY.md](docs/SYNOLOGY_CLUSTER_TOPOLOGY.md).
