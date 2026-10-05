import { useMemo } from 'react'
import { store } from '../../wailsjs/go/models'
import type { App } from '../data'

const stateLabel: Record<string, string> = {
  ok: 'connected', connecting: 'connecting…', error: 'error', auth: 'login required', idle: 'disabled',
}

type Props = {
  statuses: store.ContextStatus[]
  apps: Map<string, App>
  version: number
  selected: Set<string>
  onToggle: (id: string, multi: boolean) => void
  onAdd: () => void
  onEdit: (id: string) => void
  onLogin: (id: string) => void
  onReconnect: (id: string) => void
  onImport: () => void
  theme: string
  onTheme: () => void
}

export function Sidebar(p: Props) {
  const counts = useMemo(() => {
    const m = new Map<string, { n: number; err: number; warn: number }>()
    for (const a of p.apps.values()) {
      let c = m.get(a.ctx)
      if (!c) m.set(a.ctx, (c = { n: 0, err: 0, warn: 0 }))
      c.n++
      if (a.severity === 2) c.err++
      else if (a.severity === 1) c.warn++
    }
    return m
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p.version])

  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="logo">A</div>
        ArgoDeck
      </div>
      <div className="section-title">
        <span>Argo CD instances</span>
        {p.selected.size > 0 && (
          <a style={{ color: '#7fd3dc', textTransform: 'none', letterSpacing: 0 }} onClick={() => p.onToggle('', false)}>
            all
          </a>
        )}
      </div>
      <div className="contexts">
        {p.statuses.length === 0 && (
          <div style={{ padding: '8px 16px', color: '#7f97a5', lineHeight: 1.5 }}>
            No instances yet. Add one or import from the <code>argocd</code> CLI.
          </div>
        )}
        {p.statuses.map((s) => {
          const c = counts.get(s.id)
          return (
            <div
              key={s.id}
              className={'ctx' + (p.selected.has(s.id) ? ' active' : '')}
              onClick={(e) => p.onToggle(s.id, e.metaKey || e.ctrlKey || e.shiftKey)}
              title={s.server + (s.version ? `\nArgo CD ${s.version}` : '') + (s.user ? `\nuser: ${s.user}` : '')}
            >
              <div className="row1">
                <span className={'dot ' + s.state} style={s.color && s.state === 'ok' ? { background: s.color } : undefined} />
                <span className="name">{s.name}</span>
                <span className="ctx-actions" onClick={(e) => e.stopPropagation()}>
                  <button title="Login" onClick={() => p.onLogin(s.id)}>⎆</button>
                  <button title="Reconnect" onClick={() => p.onReconnect(s.id)}>↻</button>
                  <button title="Edit" onClick={() => p.onEdit(s.id)}>⚙</button>
                </span>
              </div>
              <div className="row2">
                {s.state === 'ok' ? (
                  <>
                    <span>{(c?.n ?? 0).toLocaleString('en-US')} apps</span>
                    {!!c?.err && <span className="err">● {c.err} failing</span>}
                    {!!c?.warn && <span className="warn">● {c.warn}</span>}
                  </>
                ) : (
                  <span>{stateLabel[s.state] ?? s.state}{c?.n ? ` · ${c.n} cached apps` : ''}</span>
                )}
              </div>
              {s.state === 'auth' && (
                <div className="state-msg" style={{ color: 'var(--missing)' }}>
                  <a style={{ color: 'var(--missing)' }} onClick={(e) => { e.stopPropagation(); p.onLogin(s.id) }}>Log in →</a>
                </div>
              )}
              {s.state === 'error' && s.message && <div className="state-msg">{s.message}</div>}
            </div>
          )
        })}
      </div>
      <div className="footer">
        <button className="nav-btn primary" onClick={p.onAdd}>＋ Add instance</button>
        <button className="nav-btn" onClick={p.onImport}>⇣ Import from argocd CLI</button>
        <button className="nav-btn" onClick={p.onTheme}>{p.theme === 'dark' ? '☀ Light theme' : '☾ Dark theme'}</button>
      </div>
    </aside>
  )
}
