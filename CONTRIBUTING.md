# Contributing to Syncscope

Thanks for helping! Syncscope is a Wails v2 desktop app: a Go backend
(`app.go` bindings + `internal/*`) and a React/TypeScript frontend (`frontend/`).

## Development setup

Requirements:

- Go 1.25+
- Node 22+ (Vitest needs `^22.12`)
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`
- Linux only: `libgtk-3-dev libwebkit2gtk-4.1-dev` (Ubuntu 24.04 / Debian 13);
  build with `-tags webkit2_41`.

Check your machine with `wails doctor`.

```bash
cd frontend && npm install && cd ..
wails dev                  # hot reload (Go + Vite)
wails build                # build/bin/Syncscope.app, .exe or Linux binary
```

### Running against the mock Argo CD

`cmd/mockargo` is a fake Argo CD API server (ApplicationSet-generated apps,
watch stream, actions, local login `admin`/`admin`, fake Dex for SSO) with
realistic failures. The quickest way to get a full dev environment:

```bash
scripts/dev.sh
```

This builds `mockargo`, starts two instances (`prod` on `:8099` with 15000
apps, `staging` on `:8098` with 4000 apps) and runs `wails dev` with an isolated
config dir (`SYNCSCOPE_CONFIG_DIR`, a fresh temp dir) and the OS keychain
disabled (`SYNCSCOPE_NO_KEYRING=1`). Tune it with `PROD_APPS`, `STAGING_APPS`,
`PROD_PORT`, `STAGING_PORT`; set `KEEP_CONFIG=1` to keep the config between runs.
Extra arguments are passed to `wails dev`.

Manually:

```bash
go run ./cmd/mockargo -port 8099 -apps 15000 -name prod
SYNCSCOPE_CONFIG_DIR=/tmp/syncscope-dev SYNCSCOPE_NO_KEYRING=1 wails dev
```

## Tests

| What | Command |
|------|---------|
| Go unit tests | `go test ./...` |
| Go vet | `go vet ./...` (Linux: `-tags webkit2_41`) |
| Integration (store vs. mockargo) | `go run ./cmd/mockargo -port 8099 -apps 3000 &` then `MOCKARGO_URL=http://localhost:8099 go test ./internal/store -run Integration -race -v` |
| Frontend types | `cd frontend && npx tsc --noEmit` |
| Frontend unit tests (Vitest) | `cd frontend && npm test` (`npm run test:watch` while developing) |
| E2E vs. a real Argo CD | `ARGOCD_E2E_URL=https://localhost:8080 ARGOCD_E2E_PASSWORD=... go test ./scripts/e2e -v -count=1` |

`go vet`/`go test` of the root package need `frontend/dist` to exist (it is
embedded). If you have never built the frontend, run `mkdir -p frontend/dist &&
touch frontend/dist/.keep` or `cd frontend && npm run build`.

The E2E suite is skipped unless the `ARGOCD_E2E_*` variables are set. CI runs it
weekly (`.github/workflows/e2e.yml`) against a pinned Argo CD in a kind
cluster, with the apps from `scripts/e2e/manifests/`. To run it locally:

```bash
kind create cluster
kubectl create ns argocd
kubectl apply -n argocd --server-side -f https://raw.githubusercontent.com/argoproj/argo-cd/v3.1.0/manifests/install.yaml
kubectl apply -n argocd -f scripts/e2e/manifests/
kubectl -n argocd port-forward svc/argocd-server 8080:443 &
export ARGOCD_E2E_URL=https://localhost:8080
export ARGOCD_E2E_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)
go test ./scripts/e2e -v -count=1
```

Frontend tests live next to the code as `frontend/src/**/*.test.ts(x)` and run
in Node. Test pure logic (search, sorting, formatting); do not import modules
that load the Wails runtime (`data.ts`, components) except as `import type`.

## Adding a feature

1. **Argo CD API call** – add it to `internal/argocd` (typed structs in
   `types.go`, request in the matching file). Mirror the endpoint in
   `cmd/mockargo` so it can be developed and tested without a cluster.
2. **Backend logic** – put state, caching and bulk operations in
   `internal/store` and cover them with unit tests (and an `Integration` test
   against mockargo when it talks HTTP).
3. **Binding** – expose a method on `App` in `app.go`. Run `wails dev` (or
   `wails generate module`) to regenerate `frontend/wailsjs/`, and commit the
   generated files.
4. **UI** – components in `frontend/src/components`, shared logic in
   plain `.ts` modules with Vitest tests.
5. Update `README.md` (Features) if the change is user-visible.

Keep the app fast with tens of thousands of apps: avoid per-app API calls in
list views, patch state incrementally, and measure with `-apps 15000`.

## Commit style

- Short imperative subject (≤ 72 chars), optionally prefixed with the area:
  `store: debounce watch reconnects`, `ui: add label filter chips`,
  `ci: cache npm`.
- Explain the *why* in the body when it is not obvious.
- One logical change per commit; keep refactors separate from behaviour changes.
- Make sure `go vet ./...`, `go test ./...`, `npx tsc --noEmit` and `npm test`
  pass before opening a PR. CI runs all of them plus the integration tests and
  a Wails build on macOS, Windows and Linux.

## Pull requests

Fill in the PR template, link the issue, and add screenshots for UI changes.
By contributing you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE).

## Releasing

See [docs/RELEASING.md](docs/RELEASING.md).
