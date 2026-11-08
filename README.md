# AX Images (`ghcr.io/ghchinoy/ax-images`)

[![Release](https://github.com/ghchinoy/ax-images/actions/workflows/release.yml/badge.svg)](https://github.com/ghchinoy/ax-images/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

Multi-arch (`linux/amd64` + `linux/arm64`), digest-pinned task images for **[AX](https://github.com/google/ax)**, **[AX Experiments](https://github.com/ghchinoy/ax-experiments)**, and **[AX Central](https://github.com/ghchinoy/ax-central)**.

| Image | Runs | Secrets / Env |
|---|---|---|
| [`swarm-agent`](images/swarm-agent) | Multi-agent Gemini worker (`python:3.12-slim` + `google-genai`) reading `Task.spec.config` via loopback metadata (`127.0.0.1:8099`) | `GEMINI_API_KEY` |
| [`genkit-runner`](images/genkit-runner) | Declarative **Genkit Go** (`v1.14.0`) Dotprompt runner serving A2A gRPC (`lf.a2a.v1.A2AService`), A2A JSON-RPC, and Genkit HTTP on `:80` | `GEMINI_API_KEY` |
| [`dgem-agent`](images/dgem-agent) | **DiffusionGemma (`dgem`)** System-1 decision router (`dgem-router`) + Genkit `systemonex.SystemOne` policy evaluator (`specialist_router`, `support_triage`, `agent_step_drift`) | `DGEM_GATEWAY_URL` |
| [`agy-agent`](images/agy-agent) | **Google Antigravity (`agy`)** coding agent with the 4-stage cascading `dgem` `PreToolUse` safety gate (`tool_safety_gate.json.tmpl`) and `ax ssh` HITL approval | `GEMINI_API_KEY`, `DGEM_GATEWAY_URL` |

All images install their entrypoint at `/usr/local/bin/ax-agent` with `/usr/local/bin/ax-task-runner` **hard-linked** (`ln -f`) to it, so local harnesses (`ate-local`) that bind-mount over `/usr/local/bin/ax-task-runner` never overwrite `/usr/local/bin/ax-agent`.

## Quickstart

### 1. Pull Pinned Release Images (`Kind` / GKE / `ate-local`)
Every release tag (`v0.1.0+`) publishes multi-arch manifests to `ghcr.io/ghchinoy/ax-images/<image>` and attaches digest-pinned `*.yaml` task manifests to the [GitHub Releases](https://github.com/ghchinoy/ax-images/releases) page.

### 2. Build Locally on macOS Apple `container` (`ate-local`)
```bash
make apple-local
```
