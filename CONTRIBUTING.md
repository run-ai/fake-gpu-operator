# Contributing to Fake GPU Operator

Thank you for your interest in contributing to the Fake GPU Operator! This document provides guidelines and instructions to help you get started.

## Getting Started

### New contributors

We're excited to help you make your first contribution! Whether you're filing issues, developing features, fixing bugs, or improving documentation, we're here to support you.

Browse issues labeled [`good first issue`](https://github.com/run-ai/fake-gpu-operator/labels/good%20first%20issue) or [`help wanted`](https://github.com/run-ai/fake-gpu-operator/labels/help%20wanted) for an easy introduction.

### Developers

The main building blocks of the Fake GPU Operator live under `cmd/<component>/` and `internal/<component>/`. Key components include:

- `device-plugin` — legacy device-plugin path for advertising fake GPUs
- `dra-plugin-gpu` — DRA plugin that allocates fake GPUs via Kubernetes 1.31+ Dynamic Resource Allocation
- `topology-server` — serves per-node GPU topology to the device-plugin/DRA plugin and the simulated `nvidia-smi`
- `status-updater` — keeps topology ConfigMaps in sync with cluster state
- `status-exporter` — exposes Prometheus metrics for simulated GPU usage
- `kwok-*` — KWOK-based variants for cluster simulation without real kubelets
- `compute-domain-controller` / `compute-domain-dra-plugin` — simulated NVIDIA Compute Domains for IMEX channel testing
- `mig-faker` — simulates MIG resource scheduling
- `nvidia-smi` — fake `nvidia-smi` binary injected into GPU pods via CDI

The Helm chart lives at `deploy/fake-gpu-operator/`. End-to-end Ginkgo tests live at `test/e2e/`.

## How to Contribute

### Reporting issues

Open an issue with a clear description, steps to reproduce, and relevant environment details. Use the bug-report or enhancement template — both are structured forms.

### Improving documentation

Help us keep the docs clear and useful by fixing typos, updating outdated information, or adding examples.

### Contributing changes

- **Fork and clone** — Fork the repository and clone it to your local machine.
- **Create a branch** — Use a descriptive name, such as `feature/add-cool-thing` or `bugfix/fix-issue-123`.
- **Make changes** — Keep commits small, focused, and well-documented.
- **Update the changelog** — For behaviour-affecting changes (features, fixes, API changes), add a line to [`CHANGELOG.md`](./CHANGELOG.md) under `## [Unreleased]`. Follow the format at [keepachangelog.com](https://keepachangelog.com/en/1.1.0/). Internal changes (refactor, tests, comments) can skip this — apply the `skip-changelog` label on your PR.
- **Submit a PR** — Open a pull request and reference any relevant issues or RUN-XXXXX Jira tickets.
- **Coverage** — Add unit, integration, or end-to-end tests covering new functionality or behaviour changes.

### PR title guidelines

PR titles must follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. The format is:

```
<type>[optional scope]: <description>
```

#### Types

- **feat**: A new feature
- **fix**: A bug fix
- **docs**: Documentation only changes
- **style**: Changes that don't affect code meaning (formatting, whitespace)
- **refactor**: Code changes that neither fix a bug nor add a feature
- **perf**: Performance improvements
- **test**: Adding or updating tests
- **build**: Changes to build system or dependencies
- **ci**: Changes to CI/CD configuration
- **chore**: Other changes that don't modify src or test files
- **revert**: Reverts a previous commit

#### Scopes (optional)

Common scopes for the Fake GPU Operator:

- `device-plugin`
- `dra-plugin`
- `topology-server`
- `status-updater`
- `status-exporter`
- `kwok`
- `compute-domain`
- `mig-faker`
- `nvidia-smi`
- `chart` (Helm chart changes)
- `ci`
- `docs`
- `e2e`

#### Breaking changes

Breaking changes MUST be indicated by adding `!` after the type/scope: `feat(chart)!: rename Helm value foo`.

#### Examples

```
feat(dra-plugin): add support for compute domains
fix(nvidia-smi): list all allocated GPUs, not just the first
docs: update topology config example in README
chore(deps): bump grpc to 1.79.3
feat(chart)!: rename topology.nodePools to topology.pools
```

#### Tips

- Use the imperative mood: "add feature" not "added feature"
- Don't end with a period

### Pull request checklist

Each pull request should meet the following requirements:

- All tests pass — Run the suites locally with `make test`, and if your change touches the operator's runtime behaviour, `make e2e`.
- Test coverage — Add or update tests for any affected code.
- Documentation — Update relevant documentation (`README.md`, in-tree docs, Helm chart values comments).
- Changelog updated — If your changes warrant logging (behaviour changes, bug fixes, new features), add them to [`CHANGELOG.md`](./CHANGELOG.md) under `## [Unreleased]`. Apply the `skip-changelog` label to opt out if not needed.
- PR description — Fill out the pull request template completely.

## Build and test

### Prerequisites

- **Go** (matches `go.mod`'s declared `go` directive)
- **Docker** (Buildx)
- **kind** ([install guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)) — only needed for e2e tests
- **Helm** 3.x — only needed for installing the chart
- **kubectl**

### Common commands

```bash
make build           # build all components for the host platform
make image           # build all container images (requires Docker Buildx)
make test            # run unit / integration tests (Ginkgo)
make lint            # golangci-lint v2.1.2
```

### End-to-end tests

The e2e suite spins up a kind cluster, builds and loads all images, deploys the operator, then runs scenarios against it.

```bash
make e2e             # full run: setup → tests → teardown
make setup-e2e       # only set up the cluster
make test-e2e        # only run tests against an existing setup
make teardown-e2e    # only delete the cluster
make e2e-profiles    # exercise the profile-based config path
```

To iterate against an existing cluster, set `SKIP_SETUP=true` on `setup-e2e` and re-run `test-e2e`.

## Getting help

Need support or have a question? Open an [issue on GitHub](https://github.com/run-ai/fake-gpu-operator/issues) — for general questions use the `question` label.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](./LICENSE).
