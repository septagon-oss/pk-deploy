# pk-deploy

> Part of [PlatformKit](https://github.com/septagon-oss/platformkit) — the
> open-source Go backend for multi-tenant SaaS.

`pk-deploy` is a small, vendor-neutral deployment control-plane kernel.

It is built for the topology PlatformKit needs first, but it is intentionally
not PlatformKit-specific:

- a self-hosted control plane (a NAS, a VM, a small server) creates signed
  deployment jobs
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
the same core for PlatformKit, a personal home-lab stack, or an unrelated SaaS.

## Install

Requires Go 1.26+.

```bash
go get github.com/septagon-oss/pk-deploy@v0.1.0
```

## Topology

An example home-lab topology (the shipped manifests use it; any host that can
run containers works the same way):

```text
Home / self-hosted control plane
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
The control-plane host never needs broad cluster credentials.

## Quickstart

```bash
make verify
make example
```

## Runtime

Build the shared runtime image and tag it for whatever registry your cluster
pulls from:

```bash
docker build -t pk-deploy:latest .
```

Run the control plane with
[deploy/synology/docker-compose.yaml](deploy/synology/docker-compose.yaml)
(an example home-lab compose file) and the worker with
[deploy/kubernetes/worker.yaml](deploy/kubernetes/worker.yaml) — replace the
placeholder image reference and control-plane URL in that manifest with your
own. The control plane exposes:

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

## Verify

```bash
make verify
```

## License

Apache-2.0. See [LICENSE](LICENSE).
