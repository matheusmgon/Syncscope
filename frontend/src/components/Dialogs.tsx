import { useEffect, useState } from 'react'
import { argocd, config, kubestore, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

function Modal({ title, children, footer, onClose }: { title: string; children: React.ReactNode; footer: React.ReactNode; onClose: () => void }) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [onClose])
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <header>{title}</header>
        <div className="content">{children}</div>
        <footer>{footer}</footer>
      </div>
    </div>
  )
}

const authHelp: Record<string, string> = {
  sso: 'Opens your browser at your identity provider (Dex, Okta, Azure AD, Keycloak, Google…), just like `argocd login --sso`. The session renews itself with the refresh token.',
  password: 'Local Argo CD account (e.g. admin), just like `argocd login --username`.',
  token: 'API token of a local account or a project role (`argocd account generate-token` / `argocd proj role create-token`).',
  cli: 'Uses the token the argocd CLI already saved in ~/.config/argocd/config.',
  core: 'Like `argocd --core`: no Argo CD API server. Applications are read and synced directly through Kubernetes with your kubeconfig. Resource tree, diff, rollback and configuration need an API server.',
}

const colors = ['', '#00a2b3', '#18be94', '#0dadea', '#f4c030', '#e96d76', '#766f94']

export function ContextDialog({ initial, onClose, onSaved }: { initial?: config.Context; onClose: () => void; onSaved: (c: config.Context) => void }) {
  const [c, setC] = useState<config.Context>(
    initial ?? config.Context.createFrom({ id: '', name: '', server: '', authType: 'sso', insecure: false, ssoPort: 8085 }),
  )
  const [headers, setHeaders] = useState(Object.entries(initial?.headers ?? {}).map(([k, v]) => `${k}: ${v}`).join('\n'))
  const [test, setTest] = useState<{ ok: boolean; msg: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmDel, setConfirmDel] = useState(false)
  const set = (patch: Partial<config.Context>) => setC(config.Context.createFrom({ ...c, ...patch }))

  const build = () => {
    const h: Record<string, string> = {}
    for (const line of headers.split('\n')) {
      const i = line.indexOf(':')
      if (i > 0) h[line.slice(0, i).trim()] = line.slice(i + 1).trim()
    }
    return config.Context.createFrom({ ...c, headers: h })
  }

  const doTest = async () => {
    setBusy(true)
    setTest(null)
    try {
      setTest({ ok: true, msg: await API.TestContext(build()) })
    } catch (e) {
      setTest({ ok: false, msg: String(e) })
    }
    setBusy(false)
  }
  const save = async () => {
    setBusy(true)
    try {
      onSaved(await API.SaveContext(build()))
    } catch (e) {
      setTest({ ok: false, msg: String(e) })
    }
    setBusy(false)
  }

  return (
    <Modal
      title={initial ? `Edit ${initial.name}` : 'Add Argo CD instance'}
      onClose={onClose}
      footer={
        <>
          {initial && (
            confirmDel ? (
              <button className="btn danger" onClick={() => API.DeleteContext(initial.id).then(onClose)}>Confirm removal</button>
            ) : (
              <button className="btn ghost" style={{ color: 'var(--error)' }} onClick={() => setConfirmDel(true)}>Remove</button>
            )
          )}
          <span className="spacer" />
          <button className="btn" onClick={doTest} disabled={busy || (c.authType === 'core' ? !c.kubeContext : !c.server)}>Test connection</button>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" onClick={save} disabled={busy || (c.authType === 'core' ? !c.kubeContext : !c.server)}>Save</button>
        </>
      }
    >
      <div className="row2col">
        <div className="field">
          <label>Name</label>
          <input type="text" value={c.name} placeholder="prod-eu" onChange={(e) => set({ name: e.target.value })} autoFocus />
        </div>
        <div className="field">
          <label>Color</label>
          <div style={{ display: 'flex', gap: 6, paddingTop: 6 }}>
            {colors.map((col) => (
              <span
                key={col}
                onClick={() => set({ color: col })}
                style={{
                  width: 20, height: 20, borderRadius: '50%', cursor: 'pointer', background: col || 'var(--border)',
                  outline: c.color === col ? '2px solid var(--fg)' : 'none', outlineOffset: 2,
                }}
              />
            ))}
          </div>
        </div>
      </div>
      {c.authType === 'core' && <CoreFields c={c} set={set} />}
      {c.authType !== 'core' && <div className="field">
        <label>Server URL</label>
        <input type="text" value={c.server} placeholder="https://argocd.example.com" onChange={(e) => set({ server: e.target.value })} />
        <span className="hint">Include the root path if any (e.g. https://host/argocd).</span>
      </div>}
      <div className="field">
        <label>Authentication</label>
        <select value={c.authType} onChange={(e) => set({ authType: e.target.value })}>
          <option value="sso">SSO (OIDC / Dex)</option>
          <option value="password">Username and password (local account)</option>
          <option value="token">API token</option>
          <option value="cli">Imported from argocd CLI</option>
          <option value="core">Core mode (kubeconfig, no API server)</option>
        </select>
        <span className="hint">{authHelp[c.authType]}</span>
      </div>
      {c.authType === 'sso' && (
        <div className="row2col">
          <div className="field">
            <label>Local callback port</label>
            <input type="text" value={c.ssoPort || 8085} onChange={(e) => set({ ssoPort: parseInt(e.target.value) || 8085 })} />
            <span className="hint">argocd CLI default: 8085</span>
          </div>
          <label className="check" style={{ marginTop: 20 }}>
            <input type="checkbox" checked={!c.ssoNoOffline} onChange={(e) => set({ ssoNoOffline: !e.target.checked })} />
            request offline_access (refresh token)
          </label>
        </div>
      )}
      {c.authType !== 'core' && <details>
        <summary>TLS, certificates and headers</summary>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 12 }}>
          <label className="check">
            <input type="checkbox" checked={c.insecure} onChange={(e) => set({ insecure: e.target.checked })} />
            Skip TLS verification (--insecure)
          </label>
          <div className="field">
            <label>Custom CA (PEM file)</label>
            <input type="text" value={c.caFile ?? ''} placeholder="~/certs/ca.pem" onChange={(e) => set({ caFile: e.target.value })} />
          </div>
          <div className="row2col">
            <div className="field">
              <label>Client certificate (mTLS)</label>
              <input type="text" value={c.clientCertFile ?? ''} onChange={(e) => set({ clientCertFile: e.target.value })} />
            </div>
            <div className="field">
              <label>Client key</label>
              <input type="text" value={c.clientKeyFile ?? ''} onChange={(e) => set({ clientKeyFile: e.target.value })} />
            </div>
          </div>
          <div className="field">
            <label>Extra headers (one per line)</label>
            <textarea rows={3} value={headers} placeholder="CF-Access-Client-Id: xxx" onChange={(e) => setHeaders(e.target.value)} />
            <span className="hint">For proxies like Cloudflare Access / IAP (same as the CLI --header flag).</span>
          </div>
        </div>
      </details>}
      {test && <div className={'alert' + (test.ok ? ' ok' : '')}>{test.msg}</div>}
    </Modal>
  )
}

export function LoginDialog({ ctx, onClose, onDone }: { ctx: config.Context; onClose: () => void; onDone: () => void }) {
  const [mode, setMode] = useState(ctx.authType === 'cli' ? 'sso' : ctx.authType)
  const [user, setUser] = useState(ctx.username || 'admin')
  const [pass, setPass] = useState('')
  const [remember, setRemember] = useState(true)
  const [token, setToken] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [ssoURL, setSsoURL] = useState('')
  const [copied, setCopied] = useState(false)

  // the backend publishes the login URL so it can be reopened or copied
  useEffect(() => EventsOn('sso:url', (p: { id: string; url: string }) => { if (p.id === ctx.id) setSsoURL(p.url) }), [ctx.id])
  // closing the dialog must stop a pending browser login (it holds the callback port)
  useEffect(() => () => { API.CancelSSO(ctx.id) }, [ctx.id])

  const cancel = () => {
    if (busy && mode === 'sso') API.CancelSSO(ctx.id)
    onClose()
  }

  const go = async () => {
    setBusy(true)
    setErr('')
    setSsoURL('')
    try {
      if (mode === 'sso') await API.LoginSSO(ctx.id)
      else if (mode === 'password') await API.LoginPassword(ctx.id, user, pass, remember)
      else await API.LoginToken(ctx.id, token)
      onDone()
    } catch (e) {
      setErr(String(e))
    }
    setBusy(false)
  }

  return (
    <Modal
      title={`Log in to ${ctx.name}`}
      onClose={cancel}
      footer={
        <>
          <button className="btn ghost" onClick={() => API.Logout(ctx.id).then(onDone)}>Logout</button>
          <span className="spacer" />
          <button className="btn" onClick={cancel}>Cancel</button>
          <button className="btn primary" onClick={go} disabled={busy}>
            {busy ? (mode === 'sso' ? 'Waiting for the browser…' : 'Signing in…') : 'Sign in'}
          </button>
        </>
      }
    >
      <div className="seg">
        <button className={mode === 'sso' ? 'on' : ''} onClick={() => setMode('sso')}>SSO</button>
        <button className={mode === 'password' ? 'on' : ''} onClick={() => setMode('password')}>Username/password</button>
        <button className={mode === 'token' ? 'on' : ''} onClick={() => setMode('token')}>Token</button>
      </div>
      <div style={{ color: 'var(--fg-muted)' }}>{ctx.server}</div>
      {mode === 'sso' && <div className="help">{authHelp.sso} The callback uses <code>http://localhost:{ctx.ssoPort || 8085}/auth/callback</code>.</div>}
      {mode === 'sso' && busy && ssoURL && (
        <div className="alert ok">
          Finish the login in your browser. Didn't open, or opened in the wrong browser/profile?
          <div style={{ display: 'flex', gap: 6, marginTop: 8 }}>
            <button className="btn sm" onClick={() => API.OpenURL(ssoURL)}>Open again</button>
            <button className="btn sm" onClick={() => { navigator.clipboard?.writeText(ssoURL); setCopied(true) }}>{copied ? 'Copied ✓' : 'Copy link'}</button>
          </div>
        </div>
      )}
      {mode === 'password' && (
        <form onSubmit={(e) => { e.preventDefault(); go() }} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div className="field"><label>Username</label><input type="text" value={user} onChange={(e) => setUser(e.target.value)} autoFocus /></div>
          <div className="field"><label>Password</label><input type="password" value={pass} onChange={(e) => setPass(e.target.value)} /></div>
          <label className="check">
            <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
            Remember the password in the keychain to renew the session automatically
          </label>
          <button type="submit" hidden />
        </form>
      )}
      {mode === 'token' && (
        <div className="field">
          <label>Token</label>
          <textarea rows={4} value={token} onChange={(e) => setToken(e.target.value)} autoFocus />
          <span className="hint">Stored in the system keychain.</span>
        </div>
      )}
      {err && <div className="alert">{err}</div>}
    </Modal>
  )
}

export type ActionKind = 'sync' | 'refresh' | 'hard' | 'restart' | 'terminate' | 'delete'

const actionTitle: Record<ActionKind, string> = {
  sync: 'Sync', refresh: 'Refresh', hard: 'Hard refresh', restart: 'Restart', terminate: 'Terminate operation', delete: 'Delete',
}

export function ConfirmAction({ kind, names, onClose, onRun }: { kind: ActionKind; names: string[]; onClose: () => void; onRun: (o: argocd.SyncOptions, d: store.DeleteOptions) => void }) {
  const [del, setDel] = useState(store.DeleteOptions.createFrom({ cascade: true, policy: 'foreground' }))
  const [typed, setTyped] = useState('')
  const [o, setO] = useState<argocd.SyncOptions>(argocd.SyncOptions.createFrom({ prune: false, dryRun: false, force: false, applyOutOfSyncOnly: false }))
  const set = (patch: Partial<argocd.SyncOptions>) => setO(argocd.SyncOptions.createFrom({ ...o, ...patch }))
  const n = names.length
  const confirmWord = n === 1 ? names[0].split(' / ').pop()! : `delete ${n}`
  const blocked = kind === 'delete' && typed.trim() !== confirmWord
  return (
    <Modal
      title={`${actionTitle[kind]} ${n} ${n === 1 ? 'application' : 'applications'}`}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className={'btn ' + (kind === 'terminate' || kind === 'delete' || (kind === 'sync' && o.prune) ? 'danger' : 'primary')} onClick={() => !blocked && onRun(o, del)} disabled={blocked} autoFocus={kind !== 'delete'}>
            {actionTitle[kind]} {n > 1 ? `(${n})` : ''}
          </button>
        </>
      }
    >
      {kind === 'sync' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <label className="check"><input type="checkbox" checked={o.prune} onChange={(e) => set({ prune: e.target.checked })} /> Prune (delete resources removed from Git)</label>
          <label className="check"><input type="checkbox" checked={o.dryRun} onChange={(e) => set({ dryRun: e.target.checked })} /> Dry run</label>
          <label className="check"><input type="checkbox" checked={o.applyOutOfSyncOnly} onChange={(e) => set({ applyOutOfSyncOnly: e.target.checked })} /> Apply out-of-sync resources only</label>
          <label className="check"><input type="checkbox" checked={o.force} onChange={(e) => set({ force: e.target.checked })} /> Force (recreate resources)</label>
        </div>
      )}
      {kind === 'delete' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <label className="check">
            <input type="checkbox" checked={del.cascade} onChange={(e) => setDel(store.DeleteOptions.createFrom({ ...del, cascade: e.target.checked }))} />
            Cascade — also delete the Kubernetes resources managed by {n === 1 ? 'this app' : 'these apps'}
          </label>
          {del.cascade && (
            <div className="seg small">
              <button className={del.policy === 'foreground' ? 'on' : ''} onClick={() => setDel(store.DeleteOptions.createFrom({ ...del, policy: 'foreground' }))}>Foreground</button>
              <button className={del.policy === 'background' ? 'on' : ''} onClick={() => setDel(store.DeleteOptions.createFrom({ ...del, policy: 'background' }))}>Background</button>
            </div>
          )}
          {!del.cascade && <div className="help">Only the Application object is removed; workloads keep running unmanaged.</div>}
          <div className="alert">Apps generated by an ApplicationSet are re-created by it unless removed from the generator.</div>
          <div className="field">
            <label>Type <b className="mono">{confirmWord}</b> to confirm</label>
            <input type="text" value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
          </div>
        </div>
      )}
      {kind === 'restart' && <div className="help">Runs the <code>restart</code> action on every Deployment, StatefulSet, DaemonSet and Rollout of each app.</div>}
      <div style={{ maxHeight: 180, overflowY: 'auto', border: '1px solid var(--border-soft)', borderRadius: 4, padding: '6px 10px', fontFamily: 'var(--mono)', fontSize: 12 }}>
        {names.slice(0, 300).map((x, i) => <div key={i}>{x}</div>)}
        {n > 300 && <div>… and {n - 300} more</div>}
      </div>
    </Modal>
  )
}

export type Toast = { id: string; kind: 'report' | 'msg'; report?: store.ActionReport; msg?: string; ok?: boolean; detail?: string; open?: () => void }

export function Toasts({ toasts, progress, onClose }: {
  toasts: Toast[]
  progress: Map<string, { id: string; action: string; done: number; total: number }>
  onClose: (id: string) => void
}) {
  return (
    <div className="toasts">
      {[...progress.values()].map((p) => (
        <div key={p.id} className="toast running">
          <div className="t-head">{p.action} in progress…</div>
          <div className="t-body">{p.done} of {p.total}</div>
          <div className="progress"><div style={{ width: `${(p.done / Math.max(1, p.total)) * 100}%` }} /></div>
        </div>
      ))}
      {toasts.map((t) => {
        if (t.kind === 'msg') {
          return (
            <div key={t.id} className={'toast' + (t.ok ? '' : ' fail') + (t.open ? ' clickable' : '')} onClick={() => { if (t.open) { t.open(); onClose(t.id) } }}>
              <div className="t-head">{t.msg}<button className="x" onClick={(e) => { e.stopPropagation(); onClose(t.id) }}>✕</button></div>
              {t.detail && <div className="t-body" style={{ whiteSpace: 'pre-wrap' }}>{t.detail}</div>}
            </div>
          )
        }
        const r = t.report!
        const failed = r.results.filter((x) => !x.ok)
        return (
          <div key={t.id} className={'toast' + (r.failed ? ' fail' : '')}>
            <div className="t-head">
              {r.failed ? '✕' : '✓'} {r.action}: {r.total - r.failed}/{r.total} ok
              <button className="x" onClick={() => onClose(t.id)}>✕</button>
            </div>
            <div className="t-body">in {r.elapsed}{r.failed ? ` · ${r.failed} failed` : ''}</div>
            {failed.length > 0 && (
              <div className="t-fail selectable">
                {failed.slice(0, 50).map((f) => (
                  <div key={f.key}><b>{f.name}</b>: {f.error}</div>
                ))}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

function CoreFields({ c, set }: { c: config.Context; set: (p: Partial<config.Context>) => void }) {
  const [ctxs, setCtxs] = useState<kubestore.ContextView[]>([])
  useEffect(() => { API.KubeContexts().then((l) => setCtxs(l ?? [])).catch(() => setCtxs([])) }, [])
  return (
    <div className="row2col">
      <div className="field">
        <label>Kubeconfig context</label>
        <select value={c.kubeContext ?? ''} onChange={(e) => set({ kubeContext: e.target.value })}>
          <option value="">— pick a context —</option>
          {ctxs.map((k) => <option key={k.name} value={k.name}>{k.name}</option>)}
        </select>
      </div>
      <div className="field">
        <label>Argo CD namespace</label>
        <input type="text" value={c.namespace ?? 'argocd'} onChange={(e) => set({ namespace: e.target.value })} />
      </div>
    </div>
  )
}
