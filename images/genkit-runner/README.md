# Declarative Genkit Go Runner (`genkit-runner`)

Task image for **[AX Experiments — Track 2b: Declarative Genkit Go Runner](https://github.com/ghchinoy/ax-experiments/tree/main/genkit)** ([`google/ax#538`](https://github.com/google/ax/issues/538), [`genkit/TUTORIAL.md`](https://github.com/ghchinoy/ax-experiments/blob/main/genkit/TUTORIAL.md)): turns a declarative `Task.spec.config` [Dotprompt](https://genkit.dev/docs/go/dotprompt/) manifest (or `Task.spec.goal.spec`) and `Workspace` into an A2A gRPC (`lf.a2a.v1.A2AService`), A2A JSON-RPC (`/`, `/.well-known/agent-card.json`), and Genkit HTTP (`/agents/<name>`) agent server on `:80` with durable `/workspace/.genkit/snapshots` across `ax suspend` and `ax resume`.

Can also connect to **[`GoogleCloudPlatform/dgem`](https://github.com/GoogleCloudPlatform/dgem)** Streamable HTTP MCP servers (`http://192.168.64.1:8095/mcp`) declared in `Workspace.spec.mcp.servers` (see [Track 2c Recipe `02-mcp-triage-agent.sh`](https://github.com/ghchinoy/ax-experiments/blob/main/dgem/02-mcp-triage-agent.sh)).

## Task Manifest (`spec.profile` + `spec.config`)

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: genkit-triage
  atespace: default
spec:
  profile:
    image: "ghcr.io/ghchinoy/ax-images/genkit-runner@sha256:2620b7bc3dc328147b034c1210c29999589eef977e05bd027529e5d882fc4fcd"
    resources:
      limits:
        cpu: "2"
        memory: "4Gi"
  debug: true
  config:
    name: triage_agent
    model: googleai/gemini-3.8-flash
    instruction: "Inspect /workspace and summarize key findings."
    maxTurns: 8
    use:
      - name: genkit-middleware/filesystem
        config:
          rootDirectory: /workspace
          allowWriteAccess: true
      - name: genkit-middleware/skills
  env:
    - name: GEMINI_API_KEY
      value: "${GEMINI_API_KEY}"
```
