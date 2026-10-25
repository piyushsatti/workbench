# Security and credentials

- Never place secrets in chat, prompts, logs, commits, agent files, or ordinary documentation.
- Keep credentials in a secrets manager and read them from there. Do not ask the human to paste a secret.
- Do not print secret values. Redirect directly to an authorized destination or keep them in a process variable.

Pi Agent is not supported. OpenCode runs under the deny-by-default permission configuration setup seeds; loosening it needs the user's go-ahead. Never send secrets to an external model.
