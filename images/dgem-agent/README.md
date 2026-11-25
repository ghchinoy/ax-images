# DiffusionGemma System-1 Decision Router (`dgem-agent`)

Task image for **[AX Experiments — Track 2c: System-1 Decision Routing & Guardrails (`dgem` + Genkit)](https://github.com/ghchinoy/ax-experiments/tree/main/dgem)** (see [`dgem/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/dgem/GUIDE.md)) powered by **[`GoogleCloudPlatform/dgem`](https://github.com/GoogleCloudPlatform/dgem)** (`DiffusionGemma 26B-A4B` discrete-diffusion template engine and [`dgem/genkit/systemonex`](https://github.com/GoogleCloudPlatform/dgem/tree/main/genkit/systemonex)).

Bundles `/usr/local/bin/dgem-router` and the `.json.tmpl` decision policy library in `/opt/dgem/templates/` (`specialist_router.json.tmpl`, `support_triage.json.tmpl`, `agent_step_drift.json.tmpl`, `tool_safety_gate.json.tmpl`), emitting structured `RouteDecision` JSON to stdout and `msg="genkit otel span"` trace lines to `/proc/1/fd/2`.

## Task Manifest (`spec.profile`)

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: dgem-router
  atespace: default
spec:
  profile:
    image: "ghcr.io/ghchinoy/ax-images/dgem-agent@sha256:e77bf4499b3548abaa4801a4ad80c1c1bf8ed3bbc7f85c245c0e4e05baaae5e1"
    resources:
      limits:
        cpu: "1"
        memory: "2Gi"
  debug: true
  env:
    - name: DGEM_GATEWAY_URL
      value: "http://192.168.64.1:8095"
    - name: DGEM_MODEL
      value: "diffgemma-26b-a4b-it-q4"
    - name: AX_TARGETS
      value: "specialist-billing,specialist-security,specialist-engineering"
```
