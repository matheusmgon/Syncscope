import { useEffect, useState } from 'react'
import { store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { ago } from './Status'

// Deploy history with the commit behind each deploy, and rollback.
export function HistoryView({ appKey, autoSync, notify }: { appKey: string; autoSync: boolean; notify: (m: string, ok: boolean) => void }) {
  const [rows, setRows] = useState<store.HistoryEntry[] | null>(null)
  const [err, setErr] = useState('')
  const [confirmId, setConfirmId] = useState<number | null>(null)
  const [prune, setPrune] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancel = false
    setRows(null)
    API.History(appKey)
      .then((r) => !cancel && setRows(r ?? []))
      .catch((e) => !cancel && setErr(String(e)))
    return () => { cancel = true }
  }, [appKey])

  const rollback = async (id: number) => {
    setBusy(true)
    try {
      await API.Rollback(appKey, id, prune, false)
      notify(`Rollback to deploy #${id} started`, true)
      setConfirmId(null)
    } catch (e) {
      notify(`Rollback failed: ${e}`, false)
    }
    setBusy(false)
  }

  if (err) return <div className="alert">{err}</div>
  if (!rows) return <div className="help">Loading history and commit details…</div>
  if (!rows.length) return <div className="help">No deploys recorded yet.</div>

  return (
    <>
      {autoSync && (
        <div className="alert" style={{ marginBottom: 12, background: 'var(--warning-bg)', color: 'var(--warning-fg)' }}>
          Auto-sync is enabled: Argo CD refuses rollbacks (it would immediately sync back to Git). Disable auto-sync first, or revert the commit in Git.
        </div>
      )}
      <div className="history">
        {rows.map((h) => {
          const [title, ...body] = (h.message || '').split('\n')
          return (
            <div key={h.id} className={'hist' + (h.current ? ' current' : '')}>
              <div className="hist-id">#{h.id}</div>
              <div className="hist-main">
                <div className="hist-title">
                  {title || <span className="muted-sm">{h.metaError ? 'commit details unavailable' : 'no commit message'}</span>}
                  {h.current && <span className="badge" style={{ marginLeft: 8 }}>current</span>}
                </div>
                {body.join('\n').trim() && <div className="hist-body">{body.join('\n').trim()}</div>}
                <div className="muted-sm">
                  <span className="mono">{h.revision.slice(0, 12)}</span>
                  {h.author && <> · {h.author}</>}
                  {h.date && <> · committed {ago(h.date)} ago</>}
                  {' · '}deployed {h.deployedAt} ({ago(h.deployedAt)} ago)
                </div>
                <div className="muted-sm mono" style={{ fontSize: 11 }}>{h.source}{h.path ? ` · ${h.path}` : ''}{h.target ? ` @ ${h.target}` : ''}</div>
                {h.metaError && <div className="muted-sm" title={h.metaError}>({h.metaError.slice(0, 120)})</div>}
              </div>
              <div className="hist-actions">
                {!h.current && confirmId !== h.id && (
                  <button className="btn sm" disabled={autoSync} onClick={() => setConfirmId(h.id)}>↶ Rollback</button>
                )}
                {confirmId === h.id && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'flex-end' }}>
                    <label className="check"><input type="checkbox" checked={prune} onChange={(e) => setPrune(e.target.checked)} /> prune</label>
                    <div style={{ display: 'flex', gap: 6 }}>
                      <button className="btn sm" onClick={() => setConfirmId(null)}>Cancel</button>
                      <button className="btn sm danger" disabled={busy} onClick={() => rollback(h.id)}>Roll back to #{h.id}</button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </>
  )
}
