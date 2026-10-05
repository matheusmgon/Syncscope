import { useMemo, useRef } from 'react'
import logo from '../assets/logo.svg'
import { ago } from './Status'
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
  onSettings: () => void
  onEdit: (id: string) => void
  onLogin: (id: string) => void
  onReconnect: (id: string) => void
  theme: string
  onTheme: () => void
  collapsed: boolean
  onCollapse: (collapsed: boolean) => void
  width: number
  onResize: (width: number) => void
}

export const SIDEBAR_MIN = 180
export const SIDEBAR_MAX = 520
export const SIDEBAR_DEFAULT = 236

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

  const dragging = useRef(false)
  const startResize = (e: React.MouseEvent) => {
    e.preventDefault()
    dragging.current = true
    const startX = e.clientX
    const startW = p.width
    document.body.classList.add('resizing')
    const move = (ev: MouseEvent) => {
      const w = startW + ev.clientX - startX
      // dragging far to the left closes the sidebar, like most desktop apps
      if (w < SIDEBAR_MIN - 60) {
        stop()
        p.onCollapse(true)
        return
      }
      p.onResize(Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, w)))
    }
    const stop = () => {
      dragging.current = false
      document.body.classList.remove('resizing')
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', stop)
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', stop)
  }

  if (p.collapsed) {
    return (
      <aside className="sidebar rail">
        <button className="rail-btn toggle" title="Show sidebar (⌘B)" onClick={() => p.onCollapse(false)}>»</button>
        <div className="contexts">
          {p.statuses.map((s) => {
            const c = counts.get(s.id)
            return (
              <div
                key={s.id}
                className={'rail-ctx' + (p.selected.has(s.id) ? ' active' : '')}
                title={`${s.name} — ${stateLabel[s.state] ?? s.state}${c ? ` · ${c.n} apps${c.err ? `, ${c.err} failing` : ''}` : ''}\n${s.server}`}
                onClick={(e) => (s.state === 'auth' ? p.onLogin(s.id) : p.onToggle(s.id, e.metaKey || e.ctrlKey || e.shiftKey))}
              >
                <span className="rail-initial" style={s.color ? { borderColor: s.color } : undefined}>{s.name.slice(0, 2)}</span>
                <span className={'dot ' + s.state} />
                {!!c?.err && <span className="rail-err">{c.err > 999 ? '999+' : c.err}</span>}
              </div>
            )
          })}
        </div>
        <div className="footer">
          <button className="rail-btn" title="Settings" onClick={p.onSettings}>⚙</button>
          <button className="rail-btn" title={p.theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'} onClick={p.onTheme}>{p.theme === 'dark' ? '☀' : '☾'}</button>
        </div>
      </aside>
    )
  }

  return (
    <aside className="sidebar" style={{ width: p.width }}>
      <div
        className="resizer"
        onMouseDown={startResize}
        onDoubleClick={() => p.onResize(SIDEBAR_DEFAULT)}
        title="Drag to resize · double-click to reset"
      />
      <div className="brand">
        <img className="logo" src={logo} alt="" />
        <span style={{ flex: 1 }}>Syncscope</span>
        <button className="rail-btn toggle" title="Hide sidebar (⌘B)" onClick={() => p.onCollapse(true)}>«</button>
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
            No instances yet. <a style={{ color: '#7fd3dc' }} onClick={p.onSettings}>Open Settings</a> to add one or import from the <code>argocd</code> CLI.
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
                  <span>{stateLabel[s.state] ?? s.state}{c?.n ? ` · ${c.n.toLocaleString('en-US')} cached apps` : ''}{s.cachedAt ? ` (${ago(s.cachedAt)} old)` : ''}</span>
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
        <div className="footer-icons">
          <button className="rail-btn" title="Settings — instances, appearance, Argo CD configuration" onClick={p.onSettings}>⚙</button>
          <span className="spacer" />
          <button className="rail-btn" title={p.theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'} onClick={p.onTheme}>{p.theme === 'dark' ? '☀' : '☾'}</button>
        </div>
      </div>
    </aside>
  )
}
