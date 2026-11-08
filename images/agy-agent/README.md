# Antigravity + `dgem` Safety Gate (`agy-agent`)

Task image for [Track 2d (`agy`)](https://github.com/ghchinoy/ax-experiments/tree/main/agy): bundles `/opt/dgem/agy-safety-gate/dgem_gate.py`, `sdk_demo.py`, and `/opt/dgem/templates/tool_safety_gate.json.tmpl` with the 4-stage cascading `PreToolUse` safety gate (`Stage 0 static -> Stage 1 dgem 2-slot -> Stage 2 Gemini -> Stage 3 file HITL over ax ssh`).

```yaml
spec:
  image: "ghcr.io/ghchinoy/ax-images/agy-agent@sha256:904b3b71f6bf7559d8562ea5875a0871e643e0a0ae5b844b634d55b518bbdfb1"
  debug: true
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
```
