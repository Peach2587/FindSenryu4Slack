# FindSenryu4Slack

Slack bot that detects senryu (5-7-5 syllable poems) in messages and notifies users.

## Commit conventions

Every commit message MUST start with a Conventional Commits prefix:

- `feat:` — a new feature
- `fix:` — a bug fix
- `docs:` — documentation only
- `refactor:` — code change that neither fixes a bug nor adds a feature
- `test:` — adding or fixing tests
- `chore:` — build, tooling, or maintenance

Example: `feat: broadcast senryu reply to channel`

## Remotes and pushing

This repository has two remotes:

- `origin` — the primary development remote. Do all normal development here.
- `urabeya` — the upstream/deployment remote.

Push to `origin` by default. Only push to `urabeya` when the user explicitly
instructs you to. Never push to `urabeya` on your own initiative.
