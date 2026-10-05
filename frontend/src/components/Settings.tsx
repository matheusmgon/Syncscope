import { useEffect, useState } from 'react'
import { argocd, config, kubestore, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'

type Tab = 'instances' | 'appearance' | 'argocd' | 'kube'

const authLabel: Record<string, string> = { sso: 'SSO', password: 'Username/password', token: 'API token', cli: 'argocd CLI' }
const stateLabel: Record<string, string> = { ok: 'connected', connecting: 'connecting…', error: 'error', auth: 'login required', idle: 'disabled' }

type Props = {
  statuses: store.ContextStatus[]
  contexts: config.Context[]
  theme: string
  setTheme: (t: string) => void
  view: 'list' | 'tiles'
  setView: (v: 'list' | 'tiles') => void
  onClose: () => void
  onAdd: () => void
  onEdit: (c: config.Context) => void
  onLogin: (c: config.Context) => void
  notify: (msg: string, ok: boolean) => void
  refreshContexts: () => void
  initialTab?: Tab
}

export function Settings(p: Props) {
  const [tab, setTab] = useState<Tab>(p.initialTab ?? 'instances')
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && p.onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [p])

  const importCLI = async () => {
    try {
      const n = await API.ImportCLI()
      p.notify(n ? `Imported ${n} context(s) from the argocd CLI` : 'No contexts found in the argocd CLI config', n > 0)
      p.refreshContexts()
    } catch (e) {
      p.notify(`Could not read ${await API.CLIConfigPath()}: ${e}`, false)
    }
  }

  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && p.onClose()}>
      <div className="modal settings">
        <header style={{ display: 'flex', alignItems: 'center' }}>
          Settings
          <button className="x" onClick={p.onClose}>✕</button>
        </header>
        <div className="settings-body">
          <nav className="settings-nav">
            <a className={tab === 'instances' ? 'on' : ''} onClick={() => setTab('instances')}>Argo CD instances</a>
            <a className={tab === 'argocd' ? 'on' : ''} onClick={() => setTab('argocd')}>Argo CD configuration</a>
            <a className={tab === 'kube' ? 'on' : ''} onClick={() => setTab('kube')}>Kubernetes clusters</a>
            <a className={tab === 'appearance' ? 'on' : ''} onClick={() => setTab('appearance')}>Appearance</a>
          </nav>
          <div className="settings-content">
            {tab === 'instances' && (
              <>
                <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                  <button className="btn primary" onClick={p.onAdd}>＋ Add instance</button>
                  <button className="btn" onClick={importCLI}>⇣ Import from argocd CLI</button>
                </div>
                {p.statuses.length === 0 && <div className="help">No instances yet.</div>}
                <table className="grid">
                  <tbody>
                    {p.statuses.map((s) => {
                      const c = p.contexts.find((x) => x.id === s.id)
                      return (
                        <tr key={s.id}>
                          <td style={{ width: 20 }}><span className={'dot ' + s.state} style={s.color ? { background: s.color } : undefined} /></td>
                          <td>
                            <b>{s.name}</b>
                            <div className="muted-sm">{s.server}</div>
                          </td>
                          <td className="muted-sm">
                            {authLabel[s.authType] ?? s.authType}
                            {s.user ? ` · ${s.user}` : ''}
                            <div>{stateLabel[s.state] ?? s.state}{s.version ? ` · ${s.version}` : ''}</div>
                          </td>
                          <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                            {c && <button className="btn sm" onClick={() => p.onLogin(c)}>Log in</button>}{' '}
                            {c && <button className="btn sm" onClick={() => p.onEdit(c)}>Edit</button>}{' '}
                            <button className="btn sm" onClick={() => API.Reconnect(s.id)}>Reconnect</button>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
                <div className="help" style={{ marginTop: 12 }}>
                  Credentials are stored in the system keychain. Config file: <code>~/Library/Application Support/syncscope/config.json</code> (macOS).
                </div>
              </>
            )}
            {tab === 'appearance' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div className="field">
                  <label>Theme</label>
                  <div className="seg">
                    <button className={p.theme === 'light' ? 'on' : ''} onClick={() => p.setTheme('light')}>☀ Light</button>
                    <button className={p.theme === 'dark' ? 'on' : ''} onClick={() => p.setTheme('dark')}>☾ Dark</button>
                  </div>
                </div>
                <div className="field">
                  <label>Applications view</label>
                  <div className="seg">
                    <button className={p.view === 'list' ? 'on' : ''} onClick={() => p.setView('list')}>☰ List</button>
                    <button className={p.view === 'tiles' ? 'on' : ''} onClick={() => p.setView('tiles')}>▦ Tiles</button>
                  </div>
                </div>
                <div className="help">Shortcuts: ⌘K search · ⌘B toggle sidebar · ↑↓ / j k move · Enter open · Space select · ⌘A select all · Esc back</div>
              </div>
            )}
            {tab === 'argocd' && <ArgoConfigView statuses={p.statuses} notify={p.notify} />}
            {tab === 'kube' && <KubeContextsView notify={p.notify} />}
          </div>
        </div>
      </div>
    </div>
  )
}

type CfgTab = 'repositories' | 'projects' | 'accounts' | 'clusters' | 'settings'

/* eslint-disable @typescript-eslint/no-explicit-any */
function ArgoConfigView({ statuses, notify }: { statuses: store.ContextStatus[]; notify: (m: string, ok: boolean) => void }) {
  const usable = statuses.filter((s) => s.state === 'ok')
  const [ctx, setCtx] = useState(usable[0]?.id ?? '')
  const [tab, setTab] = useState<CfgTab>('repositories')
  const [cfg, setCfg] = useState<store.ArgoConfig | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [tick, setTick] = useState(0)
  const [repoForm, setRepoForm] = useState(false)
  const [projEdit, setProjEdit] = useState<{ name: string; create: boolean } | null>(null)
  const [token, setToken] = useState<{ account: string; value?: string } | null>(null)
  const [rename, setRename] = useState<{ server: string; name: string; labels: Record<string, string> } | null>(null)
  const reload = () => setTick((t) => t + 1)

  useEffect(() => {
    if (!ctx) return
    setLoading(true)
    setErr('')
    API.ArgoConfig(ctx)
      .then(setCfg)
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false))
  }, [ctx, tick])

  const act = async (label: string, fn: () => Promise<unknown>) => {
    try {
      await fn()
      notify(`${label}: done`, true)
      reload()
    } catch (e) {
      notify(`${label} failed: ${e}`, false)
    }
  }

  if (!usable.length) return <div className="help">Connect to an instance first.</div>
  const tabErr = cfg?.errors?.[tab]

  return (
    <>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 10 }}>
        <select className="inline" value={ctx} onChange={(e) => setCtx(e.target.value)}>
          {usable.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>
        <span className="muted-sm">{cfg?.version ? `Argo CD ${cfg.version}` : ''} · changes use your Argo CD RBAC</span>
        <span className="spacer" />
        <button className="btn sm" onClick={reload}>Reload</button>
      </div>
      <div className="tabs" style={{ borderBottom: '1px solid var(--border)', marginBottom: 10 }}>
        {(['repositories', 'projects', 'accounts', 'clusters', 'settings'] as CfgTab[]).map((t) => (
          <div key={t} className={'tab' + (tab === t ? ' active' : '')} onClick={() => setTab(t)} style={{ textTransform: 'capitalize' }}>{t}</div>
        ))}
      </div>
      {loading && <div className="help">Loading…</div>}
      {err && <div className="alert">{err}</div>}
      {tabErr && <div className="alert">{tabErr}</div>}

      {cfg && tab === 'repositories' && (
        <>
          <button className="btn sm primary" style={{ marginBottom: 8 }} onClick={() => setRepoForm(true)}>＋ Connect repository</button>
          <table className="grid selectable">
            <thead><tr><th /><th>Repository</th><th>Type</th><th>Name / project</th><th>Status</th><th /></tr></thead>
            <tbody>
              {(cfg.repositories ?? []).map((r: any, i) => {
                const cs = (r.connectionState ?? {}) as { status?: string; message?: string }
                return (
                  <tr key={i}>
                    <td><span className={'dot ' + (cs.status === 'Successful' ? 'ok' : cs.status === 'Failed' ? 'error' : 'idle')} /></td>
                    <td className="mono">{String(r.repo ?? '')}</td>
                    <td>{String(r.type ?? 'git')}</td>
                    <td>{String(r.name ?? '')}{r.project ? ` · ${r.project}` : ''}</td>
                    <td className="msg" style={{ color: cs.status === 'Failed' ? 'var(--error-fg)' : undefined }}>{cs.status}{cs.message ? `: ${cs.message}` : ''}</td>
                    <td><Confirm label="Remove" onConfirm={() => act('Remove repository', () => API.DeleteRepository(ctx, String(r.repo)))} /></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </>
      )}
      {cfg && tab === 'projects' && (
        <>
          <button className="btn sm primary" style={{ marginBottom: 8 }} onClick={() => setProjEdit({ name: '', create: true })}>＋ New project</button>
          <table className="grid selectable">
            <thead><tr><th>Project</th><th>Description</th><th>Sources</th><th>Destinations</th><th>Sync windows</th><th /></tr></thead>
            <tbody>
              {(cfg.projects ?? []).map((p: any, i) => {
                const name = p.metadata?.name as string
                const spec = p.spec ?? {}
                return (
                  <tr key={i}>
                    <td><b>{name}</b></td>
                    <td>{spec.description}</td>
                    <td className="mono" style={{ fontSize: 11 }}>{(spec.sourceRepos ?? []).join('\n')}</td>
                    <td className="mono" style={{ fontSize: 11 }}>{(spec.destinations ?? []).map((d: any) => `${d.name || d.server} → ${d.namespace}`).join('\n')}</td>
                    <td className="mono" style={{ fontSize: 11 }}>{(spec.syncWindows ?? []).map((w: any) => `${w.kind} ${w.schedule} (${w.duration})`).join('\n')}</td>
                    <td style={{ whiteSpace: 'nowrap' }}>
                      <button className="btn sm" onClick={() => setProjEdit({ name, create: false })}>Edit</button>{' '}
                      {name !== 'default' && <Confirm label="Delete" onConfirm={() => act('Delete project', () => API.DeleteProject(ctx, name))} />}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </>
      )}
      {cfg && tab === 'accounts' && (
        <table className="grid selectable">
          <thead><tr><th>Account</th><th>Enabled</th><th>Capabilities</th><th>Tokens</th><th /></tr></thead>
          <tbody>
            {(cfg.accounts ?? []).map((a: any, i) => (
              <tr key={i}>
                <td><b>{String(a.name)}</b></td>
                <td>{a.enabled ? 'yes' : 'no'}</td>
                <td>{(a.capabilities ?? []).join(', ')}</td>
                <td>
                  {(a.tokens ?? []).map((t: any) => (
                    <div key={t.id} className="mono" style={{ fontSize: 11 }}>
                      {t.id} {t.expiresAt ? `(expires ${new Date(t.expiresAt * 1000).toLocaleDateString()})` : ''}{' '}
                      <Confirm label="revoke" small onConfirm={() => act('Revoke token', () => API.DeleteToken(ctx, String(a.name), String(t.id)))} />
                    </div>
                  ))}
                </td>
                <td>{(a.capabilities ?? []).includes('apiKey') && <button className="btn sm" onClick={() => setToken({ account: String(a.name) })}>New token</button>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {cfg && tab === 'clusters' && (
        <table className="grid selectable">
          <thead><tr><th /><th>Name</th><th>Server</th><th>Version</th><th>Message</th><th /></tr></thead>
          <tbody>
            {(cfg.clusters ?? []).map((c: any, i) => {
              const cs = (c.connectionState ?? c.info?.connectionState ?? {}) as { status?: string; message?: string }
              return (
                <tr key={i}>
                  <td><span className={'dot ' + (cs.status === 'Successful' ? 'ok' : cs.status === 'Failed' ? 'error' : 'idle')} /></td>
                  <td><b>{String(c.name ?? '')}</b></td>
                  <td className="mono">{String(c.server ?? '')}</td>
                  <td>{c.info?.serverVersion}</td>
                  <td className="msg" style={{ color: 'var(--error-fg)' }}>{cs.status === 'Failed' ? cs.message : ''}</td>
                  <td style={{ whiteSpace: 'nowrap' }}>
                    <button className="btn sm" onClick={() => setRename({ server: String(c.server), name: String(c.name ?? ''), labels: c.labels ?? {} })}>Rename</button>{' '}
                    {c.server !== 'https://kubernetes.default.svc' && <Confirm label="Remove" onConfirm={() => act('Remove cluster', () => API.DeleteCluster(ctx, String(c.server)))} />}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      {cfg && tab === 'settings' && <pre className="code-block selectable">{cfg.settings}</pre>}

      {repoForm && <RepoForm ctx={ctx} onClose={() => setRepoForm(false)} onSaved={() => { setRepoForm(false); notify('Repository connected', true); reload() }} />}
      {projEdit && <ProjectEditor ctx={ctx} name={projEdit.name} create={projEdit.create} onClose={() => setProjEdit(null)} onSaved={() => { setProjEdit(null); notify('Project saved', true); reload() }} />}
      {rename && (
        <div className="modal-backdrop" style={{ zIndex: 45 }} onMouseDown={(e) => e.target === e.currentTarget && setRename(null)}>
          <div className="modal">
            <header>Rename cluster</header>
            <div className="content">
              <div className="muted-sm mono">{rename.server}</div>
              <div className="field"><label>Name</label><input type="text" value={rename.name} autoFocus onChange={(e) => setRename({ ...rename, name: e.target.value })} /></div>
              <div className="help">Apps that target the cluster by name (destination.name) must be updated too.</div>
            </div>
            <footer>
              <button className="btn" onClick={() => setRename(null)}>Cancel</button>
              <button className="btn primary" disabled={!rename.name.trim()} onClick={() => { const r = rename; setRename(null); act('Rename cluster', () => API.UpdateClusterMeta(ctx, r.server, r.name.trim(), r.labels)) }}>Save</button>
            </footer>
          </div>
        </div>
      )}
      {token && <TokenDialog ctx={ctx} account={token.account} onClose={() => { setToken(null); reload() }} />}
    </>
  )
}

function Confirm({ label, onConfirm, small }: { label: string; onConfirm: () => void; small?: boolean }) {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const t = setTimeout(() => setArmed(false), 4000)
    return () => clearTimeout(t)
  }, [armed])
  return armed
    ? <button className="btn sm danger" onClick={() => { setArmed(false); onConfirm() }}>Confirm {label.toLowerCase()}</button>
    : <button className={'btn sm' + (small ? ' ghost' : '')} onClick={() => setArmed(true)}>{label}</button>
}

function RepoForm({ ctx, onClose, onSaved }: { ctx: string; onClose: () => void; onSaved: () => void }) {
  const [r, setR] = useState({ repo: '', type: 'git', name: '', project: '', username: '', password: '', sshPrivateKey: '', insecure: false, enableOCI: false })
  const [auth, setAuth] = useState<'none' | 'https' | 'ssh'>('none')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    setErr('')
    try {
      await API.SaveRepository(ctx, argocd.RepoInput.createFrom({
        ...r, username: auth === 'https' ? r.username : '', password: auth === 'https' ? r.password : '', sshPrivateKey: auth === 'ssh' ? r.sshPrivateKey : '',
      }), false)
      onSaved()
    } catch (e) {
      setErr(String(e))
    }
    setBusy(false)
  }
  return (
    <div className="modal-backdrop" style={{ zIndex: 45 }} onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <header>Connect repository</header>
        <div className="content">
          <div className="row2col">
            <div className="field"><label>Type</label>
              <select value={r.type} onChange={(e) => setR({ ...r, type: e.target.value })}><option value="git">git</option><option value="helm">helm</option></select>
            </div>
            <div className="field"><label>Name (helm only)</label><input type="text" value={r.name} onChange={(e) => setR({ ...r, name: e.target.value })} /></div>
          </div>
          <div className="field"><label>Repository URL</label><input type="text" className="mono" value={r.repo} placeholder="https://github.com/org/repo.git or git@github.com:org/repo.git" onChange={(e) => setR({ ...r, repo: e.target.value })} autoFocus /></div>
          <div className="field"><label>Project (optional, scopes the credentials)</label><input type="text" value={r.project} onChange={(e) => setR({ ...r, project: e.target.value })} /></div>
          <div className="seg small">
            <button className={auth === 'none' ? 'on' : ''} onClick={() => setAuth('none')}>Public</button>
            <button className={auth === 'https' ? 'on' : ''} onClick={() => setAuth('https')}>HTTPS credentials</button>
            <button className={auth === 'ssh' ? 'on' : ''} onClick={() => setAuth('ssh')}>SSH key</button>
          </div>
          {auth === 'https' && (
            <div className="row2col">
              <div className="field"><label>Username</label><input type="text" value={r.username} onChange={(e) => setR({ ...r, username: e.target.value })} /></div>
              <div className="field"><label>Password / token</label><input type="password" value={r.password} onChange={(e) => setR({ ...r, password: e.target.value })} /></div>
            </div>
          )}
          {auth === 'ssh' && <div className="field"><label>SSH private key</label><textarea rows={5} className="mono" value={r.sshPrivateKey} onChange={(e) => setR({ ...r, sshPrivateKey: e.target.value })} /></div>}
          <label className="check"><input type="checkbox" checked={r.insecure} onChange={(e) => setR({ ...r, insecure: e.target.checked })} /> skip TLS / host key verification</label>
          <div className="help">Credentials are sent to Argo CD and stored there (as a Secret), not in Syncscope.</div>
          {err && <div className="alert">{err}</div>}
        </div>
        <footer>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy || !r.repo} onClick={save}>{busy ? 'Connecting…' : 'Connect'}</button>
        </footer>
      </div>
    </div>
  )
}

const newProject = `apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: my-project
spec:
  description: ""
  sourceRepos:
    - "*"
  destinations:
    - server: https://kubernetes.default.svc
      namespace: "*"
  clusterResourceWhitelist:
    - group: "*"
      kind: "*"
  # syncWindows:
  #   - kind: deny
  #     schedule: "0 22 * * *"
  #     duration: 8h
  #     applications: ["*"]
  #     manualSync: true
`

function ProjectEditor({ ctx, name, create, onClose, onSaved }: { ctx: string; name: string; create: boolean; onClose: () => void; onSaved: () => void }) {
  const [text, setText] = useState<string | null>(create ? newProject : null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!create) API.ProjectYAML(ctx, name).then(setText).catch((e) => setErr(String(e)))
  }, [ctx, name, create])
  const save = async () => {
    setBusy(true)
    setErr('')
    try {
      await API.SaveProjectYAML(ctx, text ?? '', create)
      onSaved()
    } catch (e) {
      setErr(String(e))
    }
    setBusy(false)
  }
  return (
    <div className="modal-backdrop" style={{ zIndex: 45 }} onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal" style={{ width: 'min(820px, 94vw)' }}>
        <header>{create ? 'New project' : `Edit project ${name}`}</header>
        <div className="content">
          {text === null && !err && <div className="help">Loading…</div>}
          {text !== null && <textarea className="code-block yaml-area" style={{ minHeight: '50vh' }} spellCheck={false} value={text} onChange={(e) => setText(e.target.value)} />}
          {err && <div className="alert">{err}</div>}
        </div>
        <footer>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy || text === null} onClick={save}>{busy ? 'Saving…' : create ? 'Create' : 'Save'}</button>
        </footer>
      </div>
    </div>
  )
}

function TokenDialog({ ctx, account, onClose }: { ctx: string; account: string; onClose: () => void }) {
  const [id, setId] = useState('syncscope-' + new Date().toISOString().slice(0, 10))
  const [days, setDays] = useState(90)
  const [value, setValue] = useState('')
  const [err, setErr] = useState('')
  const gen = async () => {
    try {
      setValue(await API.CreateToken(ctx, account, id, days > 0 ? days * 86400 : 0))
    } catch (e) {
      setErr(String(e))
    }
  }
  return (
    <div className="modal-backdrop" style={{ zIndex: 45 }} onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <header>New token for {account}</header>
        <div className="content">
          {!value ? (
            <>
              <div className="row2col">
                <div className="field"><label>Token id</label><input type="text" value={id} onChange={(e) => setId(e.target.value)} /></div>
                <div className="field"><label>Expires in (days, 0 = never)</label><input type="text" value={days} onChange={(e) => setDays(Number(e.target.value) || 0)} /></div>
              </div>
            </>
          ) : (
            <>
              <div className="alert ok">Copy it now — Argo CD will not show it again.</div>
              <textarea className="code-block selectable" rows={4} readOnly value={value} />
              <button className="btn sm" onClick={() => navigator.clipboard?.writeText(value)}>Copy</button>
            </>
          )}
          {err && <div className="alert">{err}</div>}
        </div>
        <footer>
          <button className="btn" onClick={onClose}>{value ? 'Done' : 'Cancel'}</button>
          {!value && <button className="btn primary" onClick={gen}>Generate</button>}
        </footer>
      </div>
    </div>
  )
}
/* eslint-enable @typescript-eslint/no-explicit-any */

function KubeContextsView({ notify }: { notify: (m: string, ok: boolean) => void }) {
  const [list, setList] = useState<kubestore.ContextView[] | null>(null)
  const [paths, setPaths] = useState<string[]>([])
  const [err, setErr] = useState('')
  const [q, setQ] = useState('')
  const reload = () => {
    API.KubeContexts().then((l) => setList(l ?? [])).catch((e) => setErr(String(e)))
    API.KubeconfigPaths().then((p) => setPaths(p ?? []))
  }
  useEffect(reload, [])
  const toggle = async (name: string, on: boolean) => {
    const next = (list ?? []).filter((c) => (c.name === name ? on : c.enabled)).map((c) => c.name)
    try {
      await API.SetKubeContexts(next)
      setList((l) => (l ?? []).map((c) => (c.name === name ? kubestore.ContextView.createFrom({ ...c, enabled: on }) : c)))
      notify(on ? `Connecting to ${name}…` : `Disconnected from ${name}`, true)
    } catch (e) {
      notify(String(e), false)
    }
  }
  const shown = (list ?? []).filter((c) => !q || c.name.toLowerCase().includes(q.toLowerCase()))
  return (
    <>
      <div className="help" style={{ marginBottom: 12 }}>
        Argo Workflows, Argo Events and Argo Rollouts are read directly from Kubernetes using your kubeconfig, like Lens does
        (auth plugins such as <code>gke-gcloud-auth-plugin</code> work). Only the contexts you enable are contacted, and only the Argo
        objects are read (plus pods, logs and events of those objects). Actions you take (promote, stop, delete…) use your own RBAC.
      </div>
      <div className="muted-sm" style={{ marginBottom: 10 }}>kubeconfig: <span className="mono">{paths.join(', ')}</span></div>
      {err && <div className="alert">{err}</div>}
      <input className="rtree-filter" placeholder="Filter contexts…" value={q} onChange={(e) => setQ(e.target.value)} style={{ marginBottom: 8 }} />
      <table className="grid">
        <tbody>
          {shown.map((c) => (
            <tr key={c.name}>
              <td style={{ width: 30 }}><input type="checkbox" checked={c.enabled} onChange={(e) => toggle(c.name, e.target.checked)} /></td>
              <td><b>{c.name}</b>{c.current && <span className="badge" style={{ marginLeft: 6 }}>current</span>}<div className="muted-sm mono">{c.server}</div></td>
              <td className="muted-sm">{c.namespace || 'default'}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {list && !list.length && <div className="help">No contexts found in the kubeconfig.</div>}
    </>
  )
}
