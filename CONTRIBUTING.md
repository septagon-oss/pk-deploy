# Contributing

**How do I send a PR that won't be ignored?**

Thanks for wanting to contribute. This page tells you how to set up local
development, run the checks, and follow the conventions that keep a PR mergeable.
Follow these and your change reviews cleanly.

## Local development: the multi-repo workspace

PlatformKit is several independently versioned repos (`pk-core`, `pk-modules`,
`pk-apps`, `pk-runtime`, `pk-tools`, …) that compose. `pk-deploy` stands mostly
alone, but if your change spans siblings, clone them into one directory and let
a Go workspace (`go.work`) resolve them from disk:

```bash
mkdir platformkit-workspace && cd platformkit-workspace

git clone https://github.com/septagon-oss/pk-deploy
git clone https://github.com/septagon-oss/pk-core
git clone https://github.com/septagon-oss/pk-shared
# ...and any other layer you need to touch.
```

Then create a `go.work` at the workspace root listing the repos you cloned:

```bash
go work init ./pk-deploy ./pk-core ./pk-shared
```

That is the whole setup. With the workspace in place, the per-repo builds and
tests resolve sibling modules from your local checkouts. Build and test each repo
from inside it (each is its own Go module) rather than running `go build ./...`
at the workspace root.

> **Published modules carry no `replace` directives.** The `go.work` file is how
> sibling modules resolve *during local development*. Outside the workspace, each
> module resolves its dependencies by version from the Go module proxy. Do not
> add `replace` directives to a module's `go.mod` to make local dev work — that
> is exactly what `go.work` is for, and a `replace` would leak into published
> builds. If you find yourself reaching for `replace`, add the repo to your
> `go.work` instead.

## Running the checks

Run the checks from inside this repo:

```bash
go build ./...
go test ./...
make verify        # the aggregate gate: test + vet + staticcheck + fitness
```

## The invariants (these are not negotiable in review)

A PR that breaks one of these will be sent back. They are cheap to follow and
they are what keeps the architecture honest.

1. **No cross-module implementation imports.** Depend only on interfaces —
   a shared port or another package's published contract. The wiring supplies
   the concrete type. If you need a capability another package has, depend on
   its interface, never its struct. See
   [add-a-module.md](https://github.com/septagon-oss/pk-docs/blob/main/docs/v0.1.0/add-a-module.md)
   for the pattern.

2. **Migrations are append-only.** Never edit an existing migration file. Add a
   new one with a higher sequence number (`0002_...`, `0003_...`). Someone has
   already run the old one.

3. **Every Go file declares its purpose (the C-14 convention).** Each file opens
   with a short comment saying what the file owns, and references the relevant
   ADR and convention. Copy the style from any existing file in this repo.

4. **Functional options are additive.** Never change the meaning of an existing
   `WithX` option; add a new one. Callers depend on the old behavior.

## Commits and PRs

- **Conventional commits, scoped to the repo.** Format:
  `type(scope): summary`. Examples:
  - `fix(pk-deploy): fail closed when the target manifest is unreadable`
  - `docs(pk-deploy): document the rollout contract`
  Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`.
- **One repo per PR.** A change targets a single repo. If your work spans repos,
  open a PR per repo and link them.
- **Keep the diff focused.** A reviewer should be able to hold the change in
  their head. Split unrelated changes.
- **Run the checks before you push.** A green `make verify` is the bar.
- **Say what you changed and why.** A short PR body that states the problem and
  the fix beats a long one that restates the diff.

## Reporting bugs and proposing features

- Bugs and feature ideas: open a GitHub issue on the relevant repo.
- Security vulnerabilities are different — **do not** open a public issue. See
  [SECURITY.md](SECURITY.md).

---

See also:
[architecture.md](https://github.com/septagon-oss/pk-docs/blob/main/docs/v0.1.0/architecture.md)
for why the port boundary exists.
