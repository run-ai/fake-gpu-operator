# Contributing to Fake GPU Operator

Thanks for your interest in contributing! This document covers the basics of getting set up, running tests, and submitting changes.

## Prerequisites

- **Go** 1.24+ (matches the version in [`go.mod`](./go.mod))
- **Docker** (Buildx)
- **kind** ([install guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)) — only needed for e2e tests
- **Helm** 3.x — only needed for installing the chart
- **kubectl**

## Project layout

- `cmd/<component>/` — entry points for each binary (device-plugin, dra-plugin-gpu, status-updater, topology-server, nvidia-smi, …)
- `internal/<component>/` — implementation per component
- `deploy/fake-gpu-operator/` — Helm chart
- `test/e2e/` — Ginkgo-based end-to-end tests, plus the `setup.sh` script that boots a kind cluster
- `Dockerfile` — multi-stage build, one target per component

## Build

```bash
make build           # build all components for the host platform
make image           # build all container images (requires Docker Buildx)
```

By default, `make build` cross-compiles for `linux/amd64`. Override `OS` / `ARCH` if you need a different target.

## Run unit tests

```bash
make test            # runs ginkgo across ./internal/... and ./cmd/...
```

## Run e2e tests

The e2e suite spins up a kind cluster, builds and loads all images, deploys the operator, then runs scenarios against it.

```bash
make e2e             # full run: setup → tests → teardown
make setup-e2e       # only set up the cluster
make test-e2e        # only run tests against an existing setup
make teardown-e2e    # only delete the cluster
```

To iterate against an existing cluster set `SKIP_SETUP=true`:

```bash
SKIP_SETUP=true make setup-e2e
make test-e2e
```

To exercise the profile-based config path:

```bash
make e2e-profiles
```

## Lint

```bash
make lint            # golangci-lint v2.1.2
```

CI runs the same lint, so failures here will fail CI.

## Commit message style

We use [Conventional Commits](https://www.conventionalcommits.org/). PR titles are squash-merged into `main`, so the **PR title** is what ends up in `git log`:

- `feat: …` — new feature
- `fix: …` — bug fix
- `docs: …` — documentation only
- `chore: …` — tooling, deps, refactors with no functional change
- `feat!: …` or `fix!: …` — breaking change (also add `release-note/breaking` label)

Reference the Jira ticket where applicable: `feat: support FOO (RUN-XXXXX)`.

## Labels and release notes

When opening a PR, please add **one** of these labels so it shows up in the right release-notes section:

| Label | Meaning |
|---|---|
| `release-note/feature` | A new user-visible capability |
| `release-note/bug` | A bug fix |
| `release-note/breaking` | A backwards-incompatible change |
| `release-note/none` | Internal change, no user-facing release note |

The default `enhancement` and `bug` labels also work — see [`.github/release.yml`](./.github/release.yml) for the full mapping.

## Pull request checklist

Before requesting review:

- [ ] PR title uses a Conventional Commits prefix
- [ ] `make test` passes locally
- [ ] `make lint` passes locally
- [ ] `README.md` updated if user-facing behavior changed
- [ ] Helm chart values / templates updated if you added a new component or flag
- [ ] One `release-note/*` label applied

## Reporting bugs

Open an [Issue](https://github.com/run-ai/fake-gpu-operator/issues/new/choose) with the bug-report template. Please include:

- Fake GPU Operator version (Helm chart / image tag)
- Kubernetes version and distribution
- Plugin mode (legacy device plugin / DRA / KWOK)
- Minimal repro (manifest + steps)

## Questions

For questions, open a [GitHub Issue](https://github.com/run-ai/fake-gpu-operator/issues) with the `question` label.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](./LICENSE).
