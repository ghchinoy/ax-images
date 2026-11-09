# DiffusionGemma System-1 Decision Router (`dgem-agent`)

Task image for **[AX Experiments — Track 2c: System-1 Decision Routing & Guardrails (`dgem` + Genkit)](https://github.com/ghchinoy/ax-experiments/tree/main/dgem)** (see [`dgem/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/dgem/GUIDE.md)) powered by **[`GoogleCloudPlatform/dgem`](https://github.com/GoogleCloudPlatform/dgem)** (`DiffusionGemma 26B-A4B` discrete-diffusion template engine and [`dgem/genkit/systemonex`](https://github.com/GoogleCloudPlatform/dgem/tree/main/genkit/systemonex)).

Bundles `/usr/local/bin/dgem-router` and the `.json.tmpl` decision policy library in `/opt/dgem/templates/` (`specialist_router.json.tmpl`, `support_triage.json.tmpl`, `agent_step_drift.json.tmpl`, `tool_safety_gate.json.tmpl`), emitting structured `RouteDecision` JSON to stdout and `msg="genkit otel span"` trace lines to `/proc/1/fd/2`.

## Task Manifest

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: dgem-router
  atespace: default
spec:
  image: "ghcr.io/ghchinoy/ax-images/dgem-agent@sha256:eda62cfd65299d0b625a674e79c550b956a90741205ece575b5ff52d81a3e021"
  debug: true
  env:
    - name: DGEM_GATEWAY_URL
      value: "http://192.168.64.1:8095"
    - name: DGEM_MODEL
      value: "diffgemma-26b-a4b-it-q4"
    - name: AX_TARGETS
      value: "specialist-billing,specialist-security,specialist-engineering"
```
