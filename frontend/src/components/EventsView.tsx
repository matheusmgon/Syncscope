import { useEffect, useState } from 'react'
import { store } from '../../wailsjs/go/models'
import { ago } from './Status'

// Kubernetes events (warnings first-class), for an app or a single resource.
export function EventsView({ load, compact }: { load: () => Promise<store.EventRow[]>; compact?: boolean }) {
  const [rows, setRows] = useState<store.EventRow[] | null>(null)
  const [err, setErr] = useState('')
  const [onlyWarn, setOnlyWarn] = useState(false)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let cancel = false
    load().then((r) => !cancel && (setRows(r ?? []), setErr(''))).catch((e) => !cancel && setErr(String(e)))
    return () => { cancel = true }
  }, [load, tick])

  // refresh every 10s while visible
  useEffect(() => {
    const t = setInterval(() => setTick((x) => x + 1), 10000)
    return () => clearInterval(t)
  }, [])

  if (err) return <div className="alert">{err}</div>
  if (!rows) return <div className="help">Loading events…</div>
  const warn = rows.filter((r) => r.type === 'Warning').length
  const shown = onlyWarn ? rows.filter((r) => r.type === 'Warning') : rows
  return (
    <>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginBottom: 8 }}>
        <label className="check"><input type="checkbox" checked={onlyWarn} onChange={(e) => setOnlyWarn(e.target.checked)} /> warnings only ({warn})</label>
        <span className="spacer" />
        <span className="muted-sm">{rows.length} events · auto-refresh 10s</span>
      </div>
      {shown.length === 0 ? (
        <div className="muted-sm">No events. Kubernetes keeps events for about an hour.</div>
      ) : (
        <table className="mini-table selectable">
          <thead><tr><th>Type</th><th>Reason</th>{!compact && <th>Object</th>}<th>Message</th><th>Count</th><th>Last seen</th></tr></thead>
          <tbody>
            {shown.map((e, i) => (
              <tr key={i} className={e.type === 'Warning' ? 'bad' : ''}>
                <td><b style={{ color: e.type === 'Warning' ? 'var(--error)' : 'var(--fg-muted)' }}>{e.type}</b></td>
                <td>{e.reason}</td>
                {!compact && <td className="mono" style={{ fontSize: 11 }}>{e.object}</td>}
                <td className="msg">{e.message}</td>
                <td>{e.count}</td>
                <td title={`first ${e.first}\nlast ${e.last}`} style={{ whiteSpace: 'nowrap' }}>{ago(e.last)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
