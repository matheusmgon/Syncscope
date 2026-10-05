// Package e2e runs the Argo CD client against a real Argo CD (see
// .github/workflows/e2e.yml, which provisions one in kind).
//
//	ARGOCD_E2E_URL=https://localhost:8080 ARGOCD_E2E_PASSWORD=... \
//	  go test ./scripts/e2e -v -count=1
//
// Optional: ARGOCD_E2E_USER (default admin), ARGOCD_E2E_APP (app to sync,
// default guestbook), ARGOCD_E2E_APPSET (default guestbook-set).
package e2e
