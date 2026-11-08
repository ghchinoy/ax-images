# AX Images (`ghcr.io/ghchinoy/ax-images`)

[![Release](https://img.shields.io/github/v/release/ghchinoy/ax-images)](https://github.com/ghchinoy/ax-images/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

Multi-arch (`linux/amd64` + `linux/arm64`), digest-pinned container images for running **[Google Genkit Go](https://genkit.dev/docs/go/)**, **[DiffusionGemma (`dgem`)](https://github.com/GoogleCloudPlatform/dgem)** System-1 decision routers, **[Google Antigravity (`agy`)](https://github.com/rakyll/ax-images/tree/main/images/antigravity)** coding agents (in both baseline and 4-stage `dgem` safety-gated modes), and multi-agent Gemini swarms on **[AX (Agent Executor)](https://github.com/google/ax)**, **[AX Experiments](https://github.com/ghchinoy/ax-experiments)**, and **[AX Central](https://github.com/ghchinoy/ax-central)**.

Built on the single-binary runner pattern from **[`rakyll/ax-images`](https://github.com/rakyll/ax-images)**, every image compiles its agent and `ax-task-runner` into `/usr/local/bin/ax-agent` with `/usr/local/bin/ax-task-runner` **hard-linked** (`ln -f`) to it ([`rakyll/ax-images#2`](https://github.com/rakyll/ax-images/pull/2)), ensuring local microVM harnesses (`ate-local`) that bind-mount over `/usr/local/bin/ax-task-runner` never clobber the bundled agent binary.

## Table of Contents

- [Image Catalog & When to Use Upstream `rakyll/ax-images`](#image-catalog--when-to-use-upstream-rakyllax-images)
- [Quick Installation](#quick-installation)
- [Usage Examples](#usage-examples)
  - [1. Declarative Genkit Go Agent (`genkit-runner`)](#1-declarative-genkit-go-agent-genkit-runner)
  - [2. Antigravity Coding Agent — Mode A: Non-`dgem` Baseline (`agy-agent`)](#2-antigravity-coding-agent--mode-a-non-dgem-baseline-agy-agent)
  - [3. Antigravity Coding Agent — Mode B: 4-Stage `dgem` Safety Gate + `ax ssh` HITL (`agy-agent`)](#3-antigravity-coding-agent--mode-b-4-stage-dgem-safety-gate--ax-ssh-hitl-agy-agent)
  - [4. DiffusionGemma System-1 Specialist Router (`dgem-agent`)](#4-diffusiongemma-system-1-specialist-router-dgem-agent)
- [Architecture & Hard-Link Runner Contract](#architecture--hard-link-runner-contract)
- [Local Development & Upstream Sync](#local-development--upstream-sync)
- [Publish & Release Process](#publish--release-process)
- [Contributing](#contributing)
- [License](#license)

---

## Image Catalog & When to Use Upstream `rakyll/ax-images`

### Images Published by This Repository (`ghcr.io/ghchinoy/ax-images/*`)

| Image | What It Runs | Required Env / Secrets | Companion Docs & Upstream Source |
|---|---|---|---|
| [`swarm-agent`](images/swarm-agent) | Multi-agent Gemini worker (`python:3.12-slim` + `google-genai`) reading `Task.spec.config` via in-sandbox loopback metadata (`127.0.0.1:8099`) or interactive `ax ssh`. | `GEMINI_API_KEY` | [`ax-experiments/swarm`](https://github.com/ghchinoy/ax-experiments/tree/main/swarm) · [`swarm/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/swarm/GUIDE.md) |
| [`genkit-runner`](images/genkit-runner) | Declarative **Genkit Go** (`v1.14.0`) Dotprompt runner serving A2A gRPC (`lf.a2a.v1.A2AService`), A2A JSON-RPC (`/`, `/.well-known/agent-card.json`), and Genkit HTTP (`/agents/<name>`) on `:80` with `/workspace/.genkit/snapshots` persistence across `ax suspend`/`ax resume`. | `GEMINI_API_KEY` | [`google/ax#538`](https://github.com/google/ax/issues/538) · [`ax-experiments/genkit`](https://github.com/ghchinoy/ax-experiments/tree/main/genkit) · [`genkit/TUTORIAL.md`](https://github.com/ghchinoy/ax-experiments/blob/main/genkit/TUTORIAL.md) |
| [`dgem-agent`](images/dgem-agent) | **DiffusionGemma (`dgem`)** System-1 decision router (`/usr/local/bin/dgem-router`) + `.json.tmpl` policy templates (`/opt/dgem/templates/`) emitting structured `RouteDecision` JSON and OpenTelemetry trace spans. | `DGEM_GATEWAY_URL` | [`GoogleCloudPlatform/dgem`](https://github.com/GoogleCloudPlatform/dgem) · [`ax-experiments/dgem`](https://github.com/ghchinoy/ax-experiments/tree/main/dgem) · [`dgem/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/dgem/GUIDE.md) |
| [`agy-agent`](images/agy-agent) | **Dual-mode Google Antigravity (`agy`)** coding agent (`antigravity-preview-09-2026` Gemini Interactions API + CLI/SDK): runs as a non-`dgem` baseline by default, or enables the 4-stage cascading `dgem` `PreToolUse` safety gate (`tool_safety_gate.json.tmpl`) + `ax ssh` HITL when `safetyGate: true` or `DGEM_GATEWAY_URL` is set. | `GEMINI_API_KEY` *(+ optional `DGEM_GATEWAY_URL`)* | [`GoogleCloudPlatform/dgem/examples/agy-safety-gate`](https://github.com/GoogleCloudPlatform/dgem/tree/main/examples/agy-safety-gate) · [`ax-experiments/agy`](https://github.com/ghchinoy/ax-experiments/tree/main/agy) · [`agy/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/agy/GUIDE.md) |

### When to Reference Upstream [`rakyll/ax-images`](https://github.com/rakyll/ax-images) (`ghcr.io/rakyll/ax-images/*`)

Use `ghcr.io/rakyll/ax-images/*` directly when running upstream agent runners that do not require Genkit, `dgem`, or swarm extensions:

| Upstream Image | When to Use It |
|---|---|
| `ghcr.io/rakyll/ax-images/claude-managed-agents` | Running Anthropic Claude Managed Agents (`ANTHROPIC_API_KEY`) on AX workspaces. |
| `ghcr.io/rakyll/ax-images/github-actions` | Running self-hosted GitHub Actions jobs (`GITHUB_TOKEN`) inside ephemeral AX sandboxes. |
| `ghcr.io/rakyll/ax-images/antigravity` | Reference upstream baseline for `antigravity-preview-09-2026` when comparing against `ghcr.io/ghchinoy/ax-images/agy-agent`. |

---

## Quick Installation

### Option A: Use Pre-Pinned Release Manifests (`Kind` / GKE / `ate-local`)

Download any release manifest from [GitHub Releases](https://github.com/ghchinoy/ax-images/releases) and apply it to your AX cluster:

```bash
curl -fsSLO https://github.com/ghchinoy/ax-images/releases/latest/download/genkit-runner.yaml
GEMINI_API_KEY="your-key" envsubst < genkit-runner.yaml | ax apply -f -
```

### Option B: Build Local `linux/arm64` Images in macOS Apple `container` (`ate-local`)

If you are running the native macOS Apple `container` substrate ([`ate-local`](https://github.com/ghchinoy/ax-experiments/tree/main/ate-local)), build all four images directly into the local Apple `container` store in one command:

```bash
git clone https://github.com/ghchinoy/ax-images.git ~/projects/ax-images
cd ~/projects/ax-images
make apple-local
```

Verify the images are registered locally:

```bash
container image list | grep ghchinoy/ax-images
```

---

## Usage Examples

### 1. Declarative Genkit Go Agent (`genkit-runner`)

Deploy a declarative Dotprompt agent with workspace filesystem and `SKILL.md` middleware:

```bash
GEMINI_API_KEY="your-key" envsubst < images/genkit-runner/task.yaml | ax apply -f -
ax resume task genkit-triage
ax logs genkit-triage
```

### 2. Antigravity Coding Agent — Mode A: Non-`dgem` Baseline (`agy-agent`)

When `safetyGate` is `false` (and `DGEM_GATEWAY_URL` is unset), `agy-agent` runs the standard Antigravity `antigravity-preview-09-2026` loop directly against `/workspace`:

```bash
GEMINI_API_KEY="your-key" envsubst < images/agy-agent/task.yaml | ax apply -f -
ax resume task agy-agent
ax logs -f agy-agent
```

### 3. Antigravity Coding Agent — Mode B: 4-Stage `dgem` Safety Gate + `ax ssh` HITL (`agy-agent`)

Enable the 4-stage cascading `PreToolUse` safety gate (`Stage 0 static -> Stage 1 DiffusionGemma 2-slot -> Stage 2 Gemini -> Stage 3 ax ssh HITL`) by setting `safetyGate: true` or `DGEM_GATEWAY_URL`:

```bash
cat <<EOF | ax apply -f -
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: agy-dgem-gated
  atespace: default
spec:
  image: "ghcr.io/ghchinoy/ax-images/agy-agent:latest"
  debug: true
  config:
    prompt: "Inspect /workspace, run tests, and fix any failing assertions."
    safetyGate: true
  env:
    - name: GEMINI_API_KEY
      value: "${GEMINI_API_KEY}"
    - name: DGEM_GATEWAY_URL
      value: "http://192.168.64.1:8095"
    - name: DGEM_GATE_STATIC
      value: "1"
    - name: DGEM_GATE_AUDIT_LOG
      value: "/workspace/.agy/audit.jsonl"
    - name: DGEM_GATE_HITL_DIR
      value: "/workspace/.agy/hitl"
EOF
ax resume task agy-dgem-gated
```

If a borderline tool call triggers Stage 3 HITL (`decision="ask"`), inspect and approve it live over `ax ssh`:

```bash
ax ssh agy-dgem-gated -- /usr/local/bin/ax-agent hitl-list --hitl-dir /workspace/.agy/hitl
ax ssh agy-dgem-gated -- /usr/local/bin/ax-agent hitl-respond \
  --hitl-dir /workspace/.agy/hitl --id <req-id> --decision allow --by "human:sre"
```

### 4. DiffusionGemma System-1 Specialist Router (`dgem-agent`)

Run a zero-shot System-1 routing evaluation inside the `dgem-router` sandbox against `dgem serve` (`:8095`):

```bash
DGEM_GATEWAY_URL="http://192.168.64.1:8095" envsubst < images/dgem-agent/task.yaml | ax apply -f -
ax resume task dgem-router
ax ssh dgem-router -- /usr/local/bin/dgem-router \
  -query "Customer double-charged on invoice INV-902 after TLS rotation"
```

---

## Architecture & Hard-Link Runner Contract

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Container Image (ghcr.io/ghchinoy/ax-images/<name>)                  │
│                                                                      │
│  /usr/local/bin/ax-agent  ◄═══ hard link (ln -f) ═══► ax-task-runner │
│         │                                                   │        │
│         │ (invoked when spec.command is empty)              │ PID 1  │
│         ▼                                                   ▼        │
│  Agent Entrypoint:                                  AX Guest Runner  │
│  • swarm-agent   -> /usr/local/bin/agent.py         • :80 /healthz   │
│  • genkit-runner -> A2A gRPC/HTTP + Genkit on :80   • :80 /readyz    │
│  • dgem-agent    -> /usr/local/bin/dgem-router      • :80 ProcessSvc │
│  • agy-agent     -> Antigravity + optional dgem     • :8099 Metadata │
│                     PreToolUse hook (dgem_gate.py)                   │
└──────────────────────────────────────────────────────────────────────┘
```

Why hard links (`ln -f`) instead of symlinks (`ln -sf`)?
- When a local harness bind-mounts a host binary onto `/usr/local/bin/ax-task-runner`, Linux mounts follow a symlink to its target (`/usr/local/bin/ax-agent`), overwriting the agent binary itself and causing a recursive fork loop.
- With a **hard link** (`ln -f`), bind-mounting over `/usr/local/bin/ax-task-runner` only shadows that directory entry while leaving `/usr/local/bin/ax-agent`'s inode untouched ([`rakyll/ax-images#2`](https://github.com/rakyll/ax-images/pull/2)).

---

## Local Development & Upstream Sync

### Prerequisites

- **Go 1.26+** (automatically switches to the toolchain declared in `go.mod`)
- **Docker** or **Podman** (for `make build-<image>` / `make push-<image>`)
- **macOS Apple `container` CLI** (`/usr/local/bin/container`, optional — used by `make apple-local`)

### Build, Test & Sync Commands

```bash
# 1. Clone the repository
git clone https://github.com/ghchinoy/ax-images.git ~/projects/ax-images
cd ~/projects/ax-images

# 2. Run Go vet and unit tests (taskrunner, agent tools, safety gate hook, genkit-runner, agy-agent)
make test

# 3. Cross-compile static Linux binaries (amd64 or arm64)
make binaries ARCH=arm64
make binaries ARCH=amd64

# 4. Sync .json.tmpl policies and safety-gate scripts from ~/projects/dgem and ~/projects/ax-experiments
make sync

# 5. Build all 4 linux/arm64 images into local macOS Apple container
make apple-local
```

---

## Publish & Release Process

### Push Individual or All Images to GHCR / Artifact Registry

```bash
# Push a single image and record its digest in bin/<name>.digest
make push-agy-agent IMAGE_REPO=ghcr.io/ghchinoy/ax-images ARCH=arm64

# Push all images and pin @sha256: digests into images/*/task.yaml and images/*/README.md
make images IMAGE_REPO=ghcr.io/ghchinoy/ax-images
make pin IMAGE_REPO=ghcr.io/ghchinoy/ax-images
```

### Cut a Tagged Multi-Arch Release

Tagging `v*` triggers the release workflow ([`hack/release.yml`](hack/release.yml) / `.github/workflows/release.yml`) to build `linux/amd64` + `linux/arm64` manifests via `docker buildx`, push to `ghcr.io/ghchinoy/ax-images/<name>`, pin `@sha256:` digests on `main`, and attach the pinned `*.yaml` manifests to the GitHub Release:

```bash
git tag -a v0.1.1 -m "ax-images v0.1.1"
git push origin v0.1.1
```

---

## Contributing

Pull requests and issues are welcome!
- For new agent images, create `cmd/<name>/main.go` (calling `taskrunner.Main()` when `taskrunner.Invoked()` is true) and `images/<name>/{Dockerfile,README.md,task.yaml}`.
- For changes to `.json.tmpl` decision templates or `dgem_gate.py`, contribute upstream to **[`GoogleCloudPlatform/dgem`](https://github.com/GoogleCloudPlatform/dgem)** or **[`ghchinoy/ax-experiments`](https://github.com/ghchinoy/ax-experiments)** first, then run `make sync && make test`.

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
