# AGENTS.md

Homelab monitoring dashboard: single Go binary with the Vue frontend embedded via `go:embed`. All commands go through `mise` (see `mise.toml`); don't run raw go/npm builds directly.

## Commands

- `mise run dev` — backend hot-dev: `go run ./cmd/dashboard -dev` (serves frontend from `web/dist` on disk)
- `mise run web:dev` — vite dev server on :5173 proxying `/api` + `/ws` to :8080; run in a separate terminal alongside `mise run dev`
- `mise run build` — builds frontend (runs `npm install`), then `go build -o bin/dashboard ./cmd/dashboard`
- `mise run test` — `go vet ./...` + `go test ./...` + `vue-tsc --noEmit` (frontend typecheck); depends on `web:build`
- `mise run lint` — gofmt check only: `test -z "$(gofmt -l cmd internal)"` (does not cover web/)
- `mise run run` — builds then runs the binary from the repo root

Full verification is `mise run lint && mise run test`. Tool versions (Go/Node) are pinned by `mise.toml`; `mise install` syncs them.

## mise bootstrap

- `bin/mise` is a committed self-bootstrapping script (generated via `mise generate bootstrap -l -w`) so contributors/CI don't need mise pre-installed. Localized data lives in `.mise/` (gitignored); regenerate with the same command when bumping mise.
- In CI: `./bin/mise install` then `./bin/mise run test` (the run command activates the tools from `mise.toml`).

## Gotchas

- `web/embed.go` does `//go:embed dist`, so **any** `go build`/`go test` fails if `web/dist` is missing. Always build the frontend first (`mise run build` or `mise run web:build`); CI fixed this ordering on purpose (see commit 9ed8f3b).
- There are currently **no Go test files**; `go test ./...` runs nothing. The real checks are `go vet`, gofmt, and `vue-tsc`.
- Module name is `dashboard` (not `github.com/...`); imports look like `dashboard/internal/...`.
- Root `dashboard.yaml` is the live runtime config and is **gitignored** — never commit it. `config/dashboard.yaml` is the committed template embedded via `go:embed` in `config/embed.go`.
- Config resolution: `-config` flag → `./dashboard.yaml` (cwd) → `$XDG_CONFIG_HOME/dashboard/dashboard.yaml` → else a sample is generated in cwd. Running binaries from the repo root picks up the generated root `dashboard.yaml`.
- Config is hot-reloaded via fsnotify on save; a `{type:"reload"}` WS message is pushed to clients.
- Comments, README, Taskfile descriptions, and config comments are written in **Chinese** — match this convention. (Taskfile is replaced by `mise.toml`.)

## Architecture

- `cmd/dashboard/main.go` — entrypoint (flags: `-config`, `-addr`, `-dev`)
- `internal/config/` — model, resolve, fsnotify hot reload
- `internal/probe/` — http/tcp/udp probes; new types register via `Register(type, factory)` (compile-time extensible)
- `internal/dsl/` — expr-lang wrapper. DSL function names are **capitalized** (`Jsonpath`, `Regex`, `Match`, `Int`, `Float`, `Str`, `Round`). Regex patterns containing `\r\n` must use double quotes (single quotes are expr raw strings).
- `internal/collector/` — aggregator; pushes snapshot every `server.interval` over WS
- `internal/ws/` — WebSocket hub (messages: `snapshot`, `reload`, `ping`)
- `internal/types/` — JSON shape shared by backend and `web/src/types.ts` (keep in sync)
- `web/` — Vue 3 + TS + Vite + ECharts; `web/embed.go` is the embed point

## Container / Docker

- Container metrics connect to any Docker Engine API-compatible socket (docker, podman compat): resolution is `container.endpoint` config > `$DOCKER_HOST` > `unix:///var/run/docker.sock`. Docker socket isn't available in CI or bare `mise run test`.
- Docker image `laxtiz/homelab-dashboard` is published by GitHub Actions on push (multi-arch amd64/arm64): git tag → `latest` + version tag; branch push → branch tag. Requires `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN` secrets.