# Swarm Agent (`swarm-agent`)

Multi-agent Gemini worker image (`python:3.12-slim` + `google-genai` + `pyyaml` + `jq`) for **[AX Experiments — Track 2a: Multi-Agent Swarm Recipes (`01`–`08`)](https://github.com/ghchinoy/ax-experiments/tree/main/swarm)** (see [`swarm/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/swarm/GUIDE.md)).

Reads `Task.spec.config` (`role`, `model`, `prompt`) from the in-sandbox loopback metadata endpoint (`http://127.0.0.1:8099/metadata/v1alpha1/ax/task`) or environment variables (`AGENT_ROLE`, `AGENT_PROMPT`, `AX_AGENT_MODEL`). When `AGENT_PROMPT` is unset on boot, `/usr/local/bin/ax-agent` exits `0` so orchestrators can invoke `/usr/local/bin/agent.py` interactively over `ax ssh`.

## Task Manifest (`spec.config`)

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: swarm-agent
  atespace: default
spec:
  image: "ghcr.io/ghchinoy/ax-images/swarm-agent@sha256:42121723b00ef2dc90d62dcae82c913c10d31d8af3d9a3a7e63bc23f7d4e61b5"
  debug: true
  config:
    role: "Security Auditor"
    model: "gemini-3.8-flash"
    prompt: "Inspect /workspace and summarize key findings."
  env:
    - name: GEMINI_API_KEY
      value: "${GEMINI_API_KEY}"
```
