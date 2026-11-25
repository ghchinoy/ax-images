# Swarm Agent (`swarm-agent`)

Multi-agent Gemini worker image (`python:3.12-slim` + `google-genai` + `pyyaml` + `jq`) for **[AX Experiments — Track 2a: Multi-Agent Swarm Recipes (`01`–`08`)](https://github.com/ghchinoy/ax-experiments/tree/main/swarm)** (see [`swarm/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/swarm/GUIDE.md)).

Reads `Task.spec.goal.spec` and `Task.spec.config` (`role`, `model`, `prompt`) from the in-sandbox loopback metadata endpoint (`http://127.0.0.1:8099/metadata/v1alpha1/ax/task`) or environment variables (`AGENT_ROLE`, `AGENT_PROMPT`, `AX_AGENT_MODEL`). When neither a boot goal/prompt nor `AGENT_PROMPT` is set on boot, `/usr/local/bin/ax-agent` exits `0` so orchestrators can invoke `/usr/local/bin/agent.py` interactively over `ax ssh`.

## Task Manifest (`spec.profile` + `spec.goal` + `spec.config`)

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: swarm-agent
  atespace: default
spec:
  profile:
    image: "ghcr.io/ghchinoy/ax-images/swarm-agent@sha256:f23d5fda508774936a323c6c15fb937957cbccf5865bc989bb325c7854b36ea5"
    resources:
      limits:
        cpu: "1"
        memory: "2Gi"
  debug: true
  goal:
    spec: "Inspect /workspace and summarize key findings."
  config:
    role: "Security Auditor"
    model: "gemini-3.8-flash"
  env:
    - name: GEMINI_API_KEY
      value: "${GEMINI_API_KEY}"
```
