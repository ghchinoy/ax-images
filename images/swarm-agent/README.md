# Swarm Agent (`swarm-agent`)

Multi-agent Gemini worker image (`python:3.12-slim` + `google-genai` + `pyyaml` + `jq`) for [AX Experiments Swarm Recipes (`01`–`08`)](https://github.com/ghchinoy/ax-experiments/tree/main/swarm).

Reads `Task.spec.config` from the in-sandbox loopback metadata endpoint (`127.0.0.1:8099/metadata/v1alpha1/ax/task`) or environment variables (`AGENT_ROLE`, `AGENT_PROMPT`, `AX_AGENT_MODEL`).

```yaml
spec:
  image: "ghcr.io/ghchinoy/ax-images/swarm-agent:latest"
  debug: true
  config:
    role: "Code Reviewer"
    model: "gemini-3.8-flash"
```
