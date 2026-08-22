# Contributing to pi-stack

Thanks for improving pi-stack. The repository contains several clients around one stateful Go server, so small changes should make the ownership and validation boundary clear.

## Before changing code

1. Read the relevant section of [AGENTS.md](AGENTS.md), especially before changing session lifecycle, event parsing, relay, worker, or git-worktree code.
2. Check the current launcher and package manifest instead of assuming a port, command, or dependency version.
3. Keep generated files generated. In particular, do not hand-edit `pi-webby-exp/src/api/types.ts`.

## Choose the smallest validation set

Run the checks for every component you touch:

| Area | Commands |
|---|---|
| Go server | `cd pi-server-exp && go test ./... -race && go vet ./... && go build ./cmd/pi-server` |
| Webby | `cd pi-webby-exp && pnpm install && pnpm typecheck && pnpm lint && pnpm test && pnpm build` |
| Desktop | `cd pi-desktop-app && pnpm install && pnpm build` |
| Android | `cd pi-companion-exp && ./gradlew :app:compileDebugKotlin && ./gradlew :app:testDebugUnitTest` |
| Shell scripts | `bash -n install-server.sh start-exp-server.sh` |

For documentation-only changes, verify all command paths, environment variable names, ports, and links against the repository. If a command cannot be run in your environment, say so in the pull request.

## Change conventions

- Explain *why* in comments when the behavior is non-obvious; avoid comments that merely restate the code.
- Preserve lock ordering and event-delivery invariants documented in [AGENTS.md](AGENTS.md).
- Treat auth, CORS, allowed roots, worker URL validation, and insecure-mode flags as security-sensitive.
- Keep public API changes synchronized across the server, Webby, Desktop, and Companion clients when applicable.
- Update the README when a user-facing command, default, port, release artifact, or supported workflow changes.
- Put future ideas in [FEATURES.md](FEATURES.md) rather than presenting them as implemented behavior.

## Pull request checklist

- [ ] The change has a focused title and description.
- [ ] Relevant tests, type checks, lint, or builds pass.
- [ ] New behavior has a regression test where practical.
- [ ] User-facing docs and examples are current.
- [ ] No secrets, local data, build outputs, or generated artifacts are included.
- [ ] The diff does not silently broaden network exposure or weaken authentication.

## Release notes

Release artifacts are published from the repository's release workflow. Before changing packaging or installer behavior, verify the release naming and install URLs in [README.md](README.md) and test the affected platform-specific script.
