# Declarative Genkit Go Runner (`genkit-runner`)

Task image for [Track 2b (`genkit`)](https://github.com/ghchinoy/ax-experiments/tree/main/genkit) ([`google/ax#538`](https://github.com/google/ax/issues/538)): turns a declarative `Task.spec.config` [Dotprompt](https://genkit.dev/docs/go/dotprompt/) manifest and `Workspace` into an A2A JSON-RPC (`/`, `/.well-known/agent-card.json`) and Genkit HTTP (`/agents/<name>`) agent server on `:80` with durable `/workspace/.genkit/snapshots` across `ax suspend` and `ax resume`.

```yaml
spec:
  image: "ghcr.io/ghchinoy/ax-images/genkit-runner@sha256:591af6440a2b1e2a2959843eb4ec60c956c80f762b8815b10c5c390df99cc57f"
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
```
