<p align="center"><img src="build/appicon.png" width="128" alt="Syncscope icon"></p>

# Syncscope for Argo Apps

**A fast, cross-platform desktop app for the Argo tools, starting with Argo CD** — every instance and cluster in one
place, instant search over thousands of applications, and failures explained instead of
hidden. Built with Go + Wails + React.

Syncscope aims to cover everything the Argo CD web UI does, plus what it lacks when you
run ApplicationSets at scale: multi-instance view, problem grouping, bulk operations and
an offline cache.

Argo CD is supported today; Argo Workflows, Rollouts and Events are on the roadmap.

> Syncscope is an independent project. It is **not affiliated with or endorsed by** the
> Argo project or the CNCF. Argo and Argo CD are trademarks of The Linux Foundation.

## Features

- **Multiple Argo CD instances** side by side, each with its own live connection.
- **Live updates** via `/api/v1/stream/applications` (no polling), with
  automatic reconnect and re-list.
- **Instant search** over an in-memory index (tens of thousands of apps,
  virtualized table). Query language:

  | Query | Meaning |
  |---|---|
  | `payments api` | free text over name, appset, project, cluster, namespace, repo, path, labels (AND) |
  | `"exact phrase"`, `-legacy` | phrase / negation (works on every term) |
  | `appset:"x"` (quoted = exact match) | `appset:x` alone is a substring match |
  | `appset:x` `project:x` `cluster:x` `ns:x` `repo:x` `rev:main` `ctx:prod` `name:x` | field filters |
  | `health:degraded` `sync:outofsync` | status |
  | `label:team=core`, `label:team` | labels |
  | `is:error` `is:warning` `is:problem` `is:running` `is:auto` `is:manual` `is:deleting` | computed state |

  Shortcuts: `⌘B` toggle sidebar · `⌘K` or `/` search · `↑/↓` or `j/k` move · `Enter` open · `Space` select ·
  `⌘A` select all results · `Esc` clear.
- **Visible errors**: every app gets a list of problems that says *what* failed
  and *why*: failed sync operation and each failed resource/hook with its
  message, `*Error` conditions, sync retries, Degraded resources (on Argo CD 3.x
  the resource tree is fetched automatically so the pod reason, e.g.
  `CrashLoopBackOff` / `ImagePullBackOff`, is shown), unreachable clusters,
  ApplicationSet generation errors, stuck deletions.
  The **Problems** tab groups identical failures across apps ("group by cause").
- **Bulk actions** with per-app results: Sync (prune / dry-run / force /
  apply-out-of-sync-only), Refresh, Hard refresh, Restart (all Deployments,
  StatefulSets, DaemonSets and Rollouts), Terminate operation. Group headers
  (by ApplicationSet, cluster, project, instance) can be selected as a whole.
- **Application view like Argo CD**: full-page detail with a **Tree** tab (resource
  graph Application → Deployment → ReplicaSet → Pod, edges highlighted in red on the
  failing path, collapse/expand, filter, "only unhealthy", zoom with ⌘+wheel and
  Fit, side panel per resource with health message, info, restart and **live logs**
  — pod or whole workload, container picker, follow, previous container, filter,
  error highlighting; double-click a pod to open its logs), plus
  **Summary** and **Resources** tabs.
- **Instant startup with an on-disk cache**: the last known state of every instance
  (apps, ApplicationSets, clusters, failure explanations) is shown immediately on
  launch — even when the server is unreachable — while the live list + watch refresh
  it in the background. Stored gzip-compressed (`0600`) next to the config.
- **Pod terminal** through the Argo CD web terminal (xterm.js), container picker.
- **Diff** live (cluster) vs desired (Git) for the whole app and per resource,
  unified with collapsed context or full manifests.
- **Kubernetes events** for the app and per resource (warnings first, auto-refresh).
- **Per-resource actions**: every action Argo CD exposes (restart, pause, resume,
  scale…), view/edit the live manifest, delete (foreground / force / orphan).
- **Sync policy**: auto-sync, prune and self-heal per app or in bulk.
- **Edit the app**: Helm values / parameters / value files / release name, Kustomize
  images and name prefix/suffix, target revision, or the full spec as YAML
  (validated by Argo CD). Changes on ApplicationSet-generated apps show whether the
  ApplicationSet will revert them (`ignoreApplicationDifferences`, `applicationsSync`).
- **History & rollback**: every deploy with its commit message, author and date;
  roll back to any previous deploy (prune optional).
- **Delete** apps (cascade foreground/background or keep resources) from the app
  page or in bulk, with typed confirmation.
- **ApplicationSet page**: generated apps, generators, spec, conditions, bulk
  sync/refresh/restart of all its apps, delete (warns about generated apps).
- **Per-tab search**: Applications, Problems, ApplicationSets and Clusters each
  keep their own query.
- **Settings** (⚙): manage instances (add, import from argocd CLI, edit, log in),
  appearance, and a read-only view of each instance's Argo CD configuration
  (repositories with connection status, projects, accounts, clusters, settings).
- **List or Tiles** for the applications page (tiles mirror the Argo CD cards).
- Collapsible / resizable sidebar (⌘B).
- Argo CD look & feel, light and dark themes.

## Authentication

Every method `argocd login` supports against an API server:

| Method | Notes |
|---|---|
| SSO | OIDC authorization code + PKCE with a local callback on `localhost:8085` (same as `argocd login --sso`). Works with the bundled Dex and external OIDC providers (`oidc.config`, honors `cliClientID`). Refresh tokens renew the session automatically. |
| Username / password | Local accounts. Optionally remembers the password to re-login on expiry. |
| API token | Account or project role tokens. |
| Import from argocd CLI | Reads `~/.config/argocd/config` (or `$ARGOCD_CONFIG_DIR`), including tokens, refresh tokens, `insecure`, `plain-text`, `grpc-web-root-path` and client certificates. |

Also supported: `--insecure`, custom CA, mTLS client certificates, extra
headers (Cloudflare Access, IAP…), root paths (`https://host/argocd`), HTTP(S)
proxies from the environment.

Secrets are stored in the OS keychain (macOS Keychain, Windows Credential
Manager, Secret Service on Linux), falling back to a `0600` file if no
keychain is available. Config lives in the user config dir (`syncscope/`).

Not supported: `--core` mode (talking to Kubernetes directly without an Argo CD
API server).

## Development

Requirements: Go 1.25+, Node 20+, Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```bash
wails dev          # hot reload
wails build        # build/bin/Syncscope.app (or .exe / Linux binary)
```

### Mock Argo CD

`cmd/mockargo` is a fake Argo CD API (apps generated by ApplicationSets, live
stream, actions, local login `admin`/`admin`, and a fake Dex for the SSO flow)
with realistic failures:

```bash
go run ./cmd/mockargo -port 8099 -apps 15000 -name prod
go run ./cmd/mockargo -port 8098 -apps 4000 -name staging

# isolated config, no keychain
SYNCSCOPE_CONFIG_DIR=/tmp/syncscope-dev SYNCSCOPE_NO_KEYRING=1 wails dev
```

### Tests

```bash
go test ./...                                                     # unit tests
MOCKARGO_URL=http://localhost:8099 go test ./internal/store -run Integration -race -v
```

## Layout

```
main.go, app.go            Wails entry point and bindings
internal/argocd            REST client, SSO (OIDC/PKCE), token renewal
internal/config            contexts, keychain secrets, argocd CLI import
internal/store             per-instance live cache, watch stream, problem detection, bulk actions
frontend/src               React UI (data.ts = in-memory store, search.ts = query language)
cmd/mockargo               fake Argo CD server for development
```

## Roadmap

### 1. Day-to-day operations — done
Terminal, sync policy, parameters / spec editing, diff, events, resource actions,
live manifest edit and delete are implemented (see Features).

Next in this area: create applications, sync of selected resources only,
sync windows, image updater integration, "open a PR instead of editing the spec".

### 2. Distribution and quality
- Signed and notarized macOS builds; Windows and Linux builds.
- Release pipeline (GitHub Actions) and in-app auto-update.
- End-to-end tests against a real Argo CD in kind, frontend tests.

### 3. Productivity
- Desktop notifications when an app starts failing / a sync fails.
- Saved searches and favorites, recent apps, command palette.
- Edit Argo CD configuration from Settings (repositories, projects, clusters, accounts).
- `--core` mode (talk to Kubernetes directly).

### 4. Other Argo projects
- **Argo Workflows** (workflows, templates, logs), **Argo Rollouts** (canary /
  blue-green status, promote, abort), **Argo Events** (event sources, sensors).

<!-- distribution & quality (phase 2) -->

## Install

Download the latest build from the GitHub **Releases** page and verify it
against `SHA256SUMS` (`shasum -a 256 -c SHA256SUMS --ignore-missing`):

- **macOS** (Apple Silicon and Intel): `Syncscope_<version>_darwin_universal.zip`
  — unzip and move `Syncscope.app` to `/Applications`. Release builds are
  signed and notarized when signing is configured; for an unsigned build,
  right-click → *Open* the first time.
- **Windows**: `Syncscope_<version>_windows_amd64_installer.exe`, or the
  portable `.zip`. Requires the WebView2 runtime (preinstalled on Windows 11;
  the installer fetches it if missing).
- **Linux** (x86-64): `Syncscope_<version>_linux_amd64.tar.gz`. Needs GTK 3 and
  WebKitGTK 4.1 (`sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0`).

Or build from source (see Development).

## Development and testing

`scripts/dev.sh` starts two mock Argo CD instances (`:8099`, `:8098`) and
`wails dev` with an isolated config and no keychain. Frontend unit tests use
Vitest (`cd frontend && npm test`). An end-to-end suite (`scripts/e2e`) runs
the API client against a real Argo CD in kind.

CI (`.github/workflows/ci.yml`) runs on every push and PR: `go vet`, unit
tests, integration tests against `cmd/mockargo`, `tsc` + Vitest, and a Wails
build on macOS, Windows and Linux. `e2e.yml` runs weekly and on demand.

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, commands and conventions.

## Releasing

Push a `v*` tag (e.g. `git tag -a v0.2.0 -m v0.2.0 && git push origin v0.2.0`).
`.github/workflows/release.yml` builds macOS (universal), Windows (NSIS
installer) and Linux, optionally signs and notarizes the macOS app, writes
`SHA256SUMS` and publishes a GitHub Release. Details and the required signing
secrets: [docs/RELEASING.md](docs/RELEASING.md).

## License

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE).
