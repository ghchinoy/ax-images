# DiffusionGemma System-1 Router (`dgem-agent`)

Task image for [Track 2c (`dgem`)](https://github.com/ghchinoy/ax-experiments/tree/main/dgem): bundles `/usr/local/bin/dgem-router` and `/opt/dgem/templates/specialist_router.json.tmpl`, emitting structured `RouteDecision` JSON and `msg="genkit otel span"` trace lines to `/proc/1/fd/2`.

```yaml
spec:
  image: "ghcr.io/ghchinoy/ax-images/dgem-agent@sha256:a077fb0079b5513aa4f2df37bc41834e7023655281127cae3011f99285f0a586"
  debug: true
  env:
    - name: DGEM_GATEWAY_URL
      value: "http://192.168.64.1:8095"
    - name: AX_TARGETS
      value: "specialist-billing,specialist-security,specialist-engineering"
```
