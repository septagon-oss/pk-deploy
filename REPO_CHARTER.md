# pk-deploy Charter

## Purpose

`pk-deploy` defines the OSS deployment-control-plane kernel used by PlatformKit
and by non-PlatformKit projects that want one-click deploys without surrendering
runtime credentials to a hosted vendor.

## In Scope

- deployment plan contracts
- signed pull-based job envelopes
- worker execution contracts
- executor registration and deterministic step execution
- evidence bundles
- Prometheus-compatible metrics
- local examples and architecture fitness tests

## Out of Scope

- hosted SaaS control plane
- browser UI
- enterprise RBAC
- approval workflow persistence
- cloud-specific executors
- Git-provider-specific release automation
- Kubernetes manifests for a specific customer

Those belong in adapters, distributions, or private extensions.

## Release Posture

This repo should stay small enough that every exported type can be reviewed in a
single sitting. Any new dependency must justify itself against the core promise:
operators get a trustworthy, generic deploy kernel that is easy to embed,
extend, inspect, and replace.
