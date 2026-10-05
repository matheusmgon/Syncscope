# ArgoDeck

Cross-platform desktop manager for Argo CD (Go + Wails + React), built for
installations with thousands of ApplicationSet-generated apps across many
Argo CD instances.

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
keychain is available. Config lives in the user config dir (`argodeck/`).

Not supported: `--core` mode (talking to Kubernetes directly without an Argo CD
API server).

## Development

Requirements: Go 1.25+, Node 20+, Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```bash
wails dev          # hot reload
wails build        # build/bin/ArgoDeck.app (or .exe / Linux binary)
```

### Mock Argo CD

`cmd/mockargo` is a fake Argo CD API (apps generated by ApplicationSets, live
stream, actions, local login `admin`/`admin`, and a fake Dex for the SSO flow)
with realistic failures:

```bash
go run ./cmd/mockargo -port 8099 -apps 15000 -name prod
go run ./cmd/mockargo -port 8098 -apps 4000 -name staging

# isolated config, no keychain
ARGODECK_CONFIG_DIR=/tmp/argodeck-dev ARGODECK_NO_KEYRING=1 wails dev
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

- Edit Argo CD configuration from Settings (repositories, projects, clusters, accounts)
- Other Argo projects: **Argo Workflows** (workflows, templates, logs), **Argo Rollouts**
  (canary / blue-green status, promote, abort), **Argo Events** (event sources, sensors)
- App icon with the Argo look

- Pod logs and Kubernetes events in the detail panel
- Live manifest diff, rollback to a history entry
- Sync windows, app deletion, editing sync policy
- Desktop notifications when an app starts failing
- Saved searches / favorites
- `--core` mode
- Code signing and notarization for distribution
