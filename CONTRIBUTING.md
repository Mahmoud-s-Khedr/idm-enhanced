# Contributing

Read PROJECT_PLAN.md, docs/QUALITY_POLICY.md and AGENTS.md first. M0 is the active
milestone. Features start after its acceptance evidence is complete.

Install the pinned Go, Node and pnpm versions in tools/versions.json. Then run:

```sh
go run ./tools/quality bootstrap
go run ./tools/quality doctor
go run ./tools/quality check
go run ./tools/quality check:full
```

`bootstrap-deps` fetches the complete pinned Go module graph and frozen pnpm dependencies for native CI. Full bootstrap also installs pinned Go executables under .tools/bin and frozen pnpm
dependencies, then installs Lefthook. It requires network access; hooks do not.
Lefthook checks a disposable Git-index snapshot without altering partially staged
work. Use `go run ./tools/quality task <name>` for focused checks. Run
`go run ./tools/quality gate-probes` to demonstrate rejection in a disposable copy.

Regenerate contracts using `go run ./tools/contract` and desktop bindings using
`go run ./tools/quality task generate`. Never edit generated output. Format Go with
gofmt and JS/TS/docs/configuration with `pnpm exec prettier --write .`.

Use pull requests with passing Required checks; a second reviewer is not required.
Do not weaken a gate to hide a failure. Record new dependency purpose, license,
maintenance and impact, and update the ADR when changing architecture.
