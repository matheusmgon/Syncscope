## What

<!-- What does this PR change? Link the issue: "Fixes #123". -->

## Why

<!-- Motivation, context, trade-offs. -->

## How it was tested

- [ ] `go vet ./...` and `go test ./...`
- [ ] Integration tests against `cmd/mockargo` (if `internal/store` or `internal/argocd` changed)
- [ ] `cd frontend && npx tsc --noEmit && npm test`
- [ ] Manually with `scripts/dev.sh` / a real Argo CD (describe below)

<!-- Screenshots or a short recording for UI changes. -->

## Checklist

- [ ] New Argo CD API calls are mirrored in `cmd/mockargo`
- [ ] Regenerated `frontend/wailsjs/` if `app.go` bindings changed
- [ ] README updated for user-visible changes
- [ ] No secrets, server URLs or tokens in code, tests or screenshots
