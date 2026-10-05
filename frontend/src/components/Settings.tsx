import { useEffect, useState } from 'react'
import { config, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'

type Tab = 'instances' | 'appearance' | 'argocd'

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
            {tab === 'argocd' && <ArgoConfigView statuses={p.statuses} />}
          </div>
        </div>
      </div>
    </div>
  )
}

type CfgTab = 'repositories' | 'projects' | 'accounts' | 'clusters' | 'settings'

function ArgoConfigView({ statuses }: { statuses: store.ContextStatus[] }) {
  const usable = statuses.filter((s) => s.state === 'ok')
  const [ctx, setCtx] = useState(usable[0]?.id ?? '')
  const [tab, setTab] = useState<CfgTab>('repositories')
  const [cfg, setCfg] = useState<store.ArgoConfig | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!ctx) return
    setLoading(true)
    setCfg(null)
    setErr('')
    API.ArgoConfig(ctx)
      .then(setCfg)
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false))
  }, [ctx])

  if (!usable.length) return <div className="help">Connect to an instance first.</div>
  const tabErr = cfg?.errors?.[tab]

  return (
    <>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 10 }}>
        <select className="inline" value={ctx} onChange={(e) => setCtx(e.target.value)}>
          {usable.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>
        <span className="muted-sm">{cfg?.version ? `Argo CD ${cfg.version}` : ''} · read-only for now</span>
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
        <table className="grid selectable">
          <thead><tr><th /><th>Repository</th><th>Type</th><th>Name / project</th><th>Status</th></tr></thead>
          <tbody>
            {(cfg.repositories ?? []).map((r, i) => {
              const cs = (r.connectionState ?? {}) as { status?: string; message?: string }
              return (
                <tr key={i}>
                  <td><span className={'dot ' + (cs.status === 'Successful' ? 'ok' : cs.status === 'Failed' ? 'error' : 'idle')} /></td>
                  <td className="mono">{String(r.repo ?? '')}</td>
                  <td>{String(r.type ?? 'git')}</td>
                  <td>{String(r.name ?? '')}{r.project ? ` · ${r.project}` : ''}</td>
                  <td className="msg" style={{ color: cs.status === 'Failed' ? 'var(--error-fg)' : undefined }}>{cs.status}{cs.message ? `: ${cs.message}` : ''}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      {cfg && tab === 'projects' && (
        <table className="grid selectable">
          <thead><tr><th>Project</th><th>Description</th><th>Sources</th><th>Destinations</th><th>Roles</th></tr></thead>
          <tbody>
            {(cfg.projects ?? []).map((p, i) => {
              const meta = (p.metadata ?? {}) as { name?: string }
              const spec = (p.spec ?? {}) as { description?: string; sourceRepos?: string[]; destinations?: { server?: string; name?: string; namespace?: string }[]; roles?: { name: string }[] }
              return (
                <tr key={i}>
                  <td><b>{meta.name}</b></td>
                  <td>{spec.description}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{(spec.sourceRepos ?? []).join('\n')}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{(spec.destinations ?? []).map((d) => `${d.name || d.server} → ${d.namespace}`).join('\n')}</td>
                  <td>{(spec.roles ?? []).map((r) => r.name).join(', ')}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      {cfg && tab === 'accounts' && (
        <table className="grid selectable">
          <thead><tr><th>Account</th><th>Enabled</th><th>Capabilities</th><th>Tokens</th></tr></thead>
          <tbody>
            {(cfg.accounts ?? []).map((a, i) => (
              <tr key={i}>
                <td><b>{String(a.name)}</b></td>
                <td>{a.enabled ? 'yes' : 'no'}</td>
                <td>{((a.capabilities as string[]) ?? []).join(', ')}</td>
                <td>{((a.tokens as unknown[]) ?? []).length}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {cfg && tab === 'clusters' && (
        <table className="grid selectable">
          <thead><tr><th /><th>Name</th><th>Server</th><th>Version</th><th>Message</th></tr></thead>
          <tbody>
            {(cfg.clusters ?? []).map((c, i) => {
              const cs = ((c.connectionState ?? (c.info as { connectionState?: object })?.connectionState) ?? {}) as { status?: string; message?: string }
              const info = (c.info ?? {}) as { serverVersion?: string }
              return (
                <tr key={i}>
                  <td><span className={'dot ' + (cs.status === 'Successful' ? 'ok' : cs.status === 'Failed' ? 'error' : 'idle')} /></td>
                  <td><b>{String(c.name ?? '')}</b></td>
                  <td className="mono">{String(c.server ?? '')}</td>
                  <td>{info.serverVersion}</td>
                  <td className="msg" style={{ color: 'var(--error-fg)' }}>{cs.status === 'Failed' ? cs.message : ''}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      {cfg && tab === 'settings' && <pre className="code-block selectable">{cfg.settings}</pre>}
    </>
  )
}
