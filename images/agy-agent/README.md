# Antigravity Coding Agent + Optional `dgem` Safety Gate (`agy-agent`)

Dual-mode Google Antigravity (`antigravity-preview-09-2026` Gemini Interactions API + `agy` CLI/SDK) task image for **[AX Experiments — Track 2d: Antigravity Coding Agents on AX](https://github.com/ghchinoy/ax-experiments/tree/main/agy)** (see [`agy/GUIDE.md`](https://github.com/ghchinoy/ax-experiments/blob/main/agy/GUIDE.md)).

Extends the upstream **[`rakyll/ax-images/antigravity`](https://github.com/rakyll/ax-images/tree/main/images/antigravity)** runner with `spec.goal.spec` support and **[`GoogleCloudPlatform/dgem`](https://github.com/GoogleCloudPlatform/dgem)** [`examples/agy-safety-gate`](https://github.com/GoogleCloudPlatform/dgem/tree/main/examples/agy-safety-gate) (`/opt/dgem/agy-safety-gate/dgem_gate.py`, `sdk_demo.py`, and [`templates/tool_safety_gate.json.tmpl`](https://github.com/GoogleCloudPlatform/dgem/blob/main/templates/tool_safety_gate.json.tmpl)).

## Two Operating Modes in One Image

### Mode A: Non-`dgem` Baseline (Default — Compatible with `ghcr.io/rakyll/ax-images/antigravity`)
When `DGEM_GATEWAY_URL` and `DGEM_GATE_STATIC` are unset (and `spec.config.safetyGate` is omitted or `false`), `agy-agent` runs the standard Antigravity Gemini Interactions loop (`antigravity-preview-09-2026`) executing `run_shell`, `read_file`, `write_file`, and `edit_file` directly inside `/workspace`:

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: agy-baseline
  atespace: default
spec:
  profile:
    image: "ghcr.io/ghchinoy/ax-images/agy-agent@sha256:2d1ae9d22688ab26926ce808c9231e8c07b019c15b0f530d5616a50956dc63a1"
    resources:
      limits:
        cpu: "2"
        memory: "4Gi"
  debug: true
  goal:
    spec: "Run the test suite, find the failing test, and fix it."
  config:
    maxTurns: 20
  env:
    - name: GEMINI_API_KEY
      value: "${GEMINI_API_KEY}"
```

### Mode B: 4-Stage Cascading `dgem` `PreToolUse` Safety Gate + `ax ssh` HITL
When `spec.config.safetyGate: true` or `DGEM_GATEWAY_URL` / `DGEM_GATE_STATIC=1` is set, every local workspace tool call (`run_shell`, `write_file`, `edit_file`, `read_file`) is intercepted before execution by the 4-stage cascading safety gate (`Stage 0 static regex -> Stage 1 dgem 2-slot DiffusionGemma -> Stage 2 Gemini System-2 tie-breaker -> Stage 3 file-based HITL over ax ssh`):

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: agy-dgem-gate
  atespace: default
spec:
  profile:
    image: "ghcr.io/ghchinoy/ax-images/agy-agent@sha256:2d1ae9d22688ab26926ce808c9231e8c07b019c15b0f530d5616a50956dc63a1"
    resources:
      limits:
        cpu: "2"
        memory: "4Gi"
  debug: true
  goal:
    spec: "Audit and remediate the payment service."
  config:
    safetyGate: true
  env:
    - name: GEMINI_API_KEY
      value: "${GEMINI_API_KEY}"
    - name: DGEM_GATEWAY_URL
      value: "http://192.168.64.1:8095"
    - name: DGEM_GATE_STATIC
      value: "1"
    - name: DGEM_GATE_CASCADE
      value: "gemini"
    - name: DGEM_GATE_AUDIT_LOG
      value: "/workspace/.agy/audit.jsonl"
    - name: DGEM_GATE_HITL_DIR
      value: "/workspace/.agy/hitl"
```
