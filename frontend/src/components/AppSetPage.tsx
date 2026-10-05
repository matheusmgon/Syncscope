import { useEffect, useMemo, useState } from 'react'
import { store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { useData } from '../data'
import { HealthIcon, StatBar, SyncIcon, ago } from './Status'
import { ProblemList } from './Problems'
import type { ActionKind } from './Dialogs'

type Props = {
  setKey: string
  ctxName: string
  left: number
  onClose: () => void
  onOpenApp: (key: string) => void
  onShowInList: (name: string) => void
  onAction: (kind: ActionKind, keys: string[]) => void
  notify: (m: string, ok: boolean) => void
}

type Tab = 'apps' | 'spec' | 'conditions'
const HEALTHS = ['Healthy', 'Progressing', 'Suspended', 'Missing', 'Degraded', 'Unknown']

export function AppSetPage(p: Props) {
  const data = useData()
  const [d, setD] = useState<store.AppSetDetail | null>(null)
  const [err, setErr] = useState('')
  const [tab, setTab] = useState<Tab>('apps')
  const [confirmDel, setConfirmDel] = useState(false)
  const [typed, setTyped] = useState('')
  const ctx = p.setKey.slice(0, p.setKey.indexOf('|'))
  const name = p.setKey.slice(p.setKey.indexOf('/') + 1)

  useEffect(() => {
    API.AppSetDetail(p.setKey).then(setD).catch((e) => setErr(String(e)))
  }, [p.setKey])

  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && !confirmDel && p.onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [p, confirmDel])

  const apps = useMemo(
    () => [...data.apps.values()].filter((a) => a.ctx === ctx && a.appSet === name).sort((a, b) => b.severity - a.severity || a.name.localeCompare(b.name)),
    [data.version, ctx, name], // eslint-disable-line react-hooks/exhaustive-deps
  )
  const keys = apps.map((a) => a.key)
  const hc: Record<string, number> = {}
  for (const a of apps) hc[a.health] = (hc[a.health] ?? 0) + 1
  const failing = apps.filter((a) => a.severity === 2).length
  const live = data.appsets.find((s) => s.key === p.setKey)
  const problems = live?.problems ?? d?.summary.problems ?? []

  const doDelete = async () => {
    try {
      await API.DeleteAppSet(p.setKey)
      p.notify(`ApplicationSet ${name} deleted`, true)
      p.onClose()
    } catch (e) {
      p.notify(`Delete failed: ${e}`, false)
    }
  }

  return (
    <div className="drawer page" style={{ left: p.left }}>
      <header>
        <h2>
          <span className="kicon app" style={{ width: 28, height: 28, fontSize: 8 }}>appset</span>
          <span className="selectable">{name}</span>
          <button className="btn ghost sm back" onClick={p.onClose}>← Back</button>
        </h2>
        <div className="meta">
          ApplicationSet · {p.ctxName}{d?.summary.namespace ? ` · ${d.summary.namespace}` : ''}
          {d?.generators?.length ? <> · generators: <b>{d.generators.join(', ')}</b></> : null}
        </div>
        <div className="tools">
          <span className="pill">{apps.length} apps</span>
          {failing > 0 && <span className="badge error">{failing} failing</span>}
          <StatBar total={apps.length} counts={HEALTHS.map((h) => [h, hc[h] ?? 0])} />
        </div>
        <div className="tools">
          <button className="btn" onClick={() => p.onShowInList(name)}>☰ Show apps in list</button>
          <button className="btn primary" disabled={!keys.length} onClick={() => p.onAction('sync', keys)}>⟳ Sync all</button>
          <button className="btn" disabled={!keys.length} onClick={() => p.onAction('refresh', keys)}>Refresh all</button>
          <button className="btn" disabled={!keys.length} onClick={() => p.onAction('restart', keys)}>↻ Restart all</button>
          <span className="spacer" />
          <button className="btn danger-outline" onClick={() => setConfirmDel(true)}>🗑 Delete ApplicationSet</button>
        </div>
        <div className="tabs detail-tabs">
          <div className={'tab' + (tab === 'apps' ? ' active' : '')} onClick={() => setTab('apps')}>Applications <span className="badge">{apps.length}</span></div>
          <div className={'tab' + (tab === 'spec' ? ' active' : '')} onClick={() => setTab('spec')}>Spec</div>
          <div className={'tab' + (tab === 'conditions' ? ' active' : '')} onClick={() => setTab('conditions')}>Conditions</div>
        </div>
      </header>
      {problems.length > 0 && (
        <div className="detail-problems selectable">
          <div className="dp-head"><b>ApplicationSet error</b></div>
          <ProblemList problems={problems} />
        </div>
      )}
      <div className="body selectable">
        {err && <div className="alert">{err}</div>}
        {tab === 'apps' && (
          <table className="grid">
            <thead><tr><th /><th /><th>Application</th><th>Destination</th><th>Problem</th><th>Last sync</th></tr></thead>
            <tbody>
              {apps.map((a) => (
                <tr key={a.key} className="click" onClick={() => p.onOpenApp(a.key)}>
                  <td style={{ width: 20 }}><HealthIcon status={a.health} /></td>
                  <td style={{ width: 20 }}><SyncIcon status={a.sync} running={a.opPhase === 'Running'} /></td>
                  <td><b>{a.name}</b></td>
                  <td className="muted-sm">{a.cluster}/{a.destNamespace}</td>
                  <td className="msg" style={{ color: a.severity === 2 ? 'var(--error-fg)' : 'var(--warning-fg)' }}>{a.problems?.[0]?.message}</td>
                  <td className="muted-sm">{a.opFinishedAt ? ago(a.opFinishedAt) : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {tab === 'apps' && !apps.length && <div className="empty">No applications are currently generated by this ApplicationSet.</div>}
        {tab === 'spec' && (d ? <pre className="code-block" style={{ maxHeight: 'none' }}>{d.spec}</pre> : <div className="help">Loading…</div>)}
        {tab === 'conditions' && (
          <table className="mini-table">
            <tbody>
              {(d?.conditions ?? []).map((c, i) => (
                <tr key={i} className={c.type === 'ErrorOccurred' && c.status === 'True' ? 'bad' : ''}>
                  <td style={{ width: 220 }}><b>{c.type}</b> = {c.status}<div className="muted-sm">{c.reason}</div><div className="muted-sm">{c.lastTransitionTime}</div></td>
                  <td className="msg">{c.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {confirmDel && (
        <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && setConfirmDel(false)}>
          <div className="modal">
            <header>Delete ApplicationSet {name}</header>
            <div className="content">
              {d?.preserve ? (
                <div className="alert ok">preserveResourcesOnDeletion is enabled: the {apps.length} generated Applications are deleted but their Kubernetes resources are kept.</div>
              ) : (
                <div className="alert">
                  This also deletes the <b>{apps.length} generated Applications</b> and, through cascade, the Kubernetes resources they manage.
                </div>
              )}
              <div className="field">
                <label>Type <b className="mono">{name}</b> to confirm</label>
                <input type="text" value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
              </div>
            </div>
            <footer>
              <button className="btn" onClick={() => setConfirmDel(false)}>Cancel</button>
              <button className="btn danger" disabled={typed.trim() !== name} onClick={doDelete}>Delete ApplicationSet</button>
            </footer>
          </div>
        </div>
      )}
    </div>
  )
}
