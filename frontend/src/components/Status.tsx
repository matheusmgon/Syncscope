import type { ReactElement } from 'react'
// Health / sync indicators modeled after the Argo CD UI icons and colors.
export const healthColor: Record<string, string> = {
  Healthy: 'var(--healthy)', Progressing: 'var(--progressing)', Degraded: 'var(--degraded)',
  Suspended: 'var(--suspended)', Missing: 'var(--missing)', Unknown: 'var(--unknown)',
}
export const syncColor: Record<string, string> = {
  Synced: 'var(--synced)', OutOfSync: 'var(--outofsync)', Unknown: 'var(--unknown)',
}

export function HealthIcon({ status }: { status: string }) {
  const c = healthColor[status] ?? 'var(--unknown)'
  let path: ReactElement
  switch (status) {
    case 'Healthy':
      path = <path fill={c} d="M8 14.5 1.9 8.6A3.7 3.7 0 0 1 8 3.6a3.7 3.7 0 0 1 6.1 5Z" />
      break
    case 'Degraded':
      path = <path fill={c} d="M8 14.5 1.9 8.6A3.7 3.7 0 0 1 7.2 3l-1.3 3.3 2.5 1.4-1.2 3.6 3.5-4.3-2.4-1.4L9.6 3a3.7 3.7 0 0 1 4.5 5.6Z" />
      break
    case 'Progressing':
      return (
        <span className="si spin" title={status}>
          <svg viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="none" stroke={c} strokeWidth="2.2" strokeDasharray="26 12" strokeLinecap="round" /></svg>
        </span>
      )
    case 'Suspended':
      path = <g fill={c}><rect x="3.5" y="2.5" width="3" height="11" rx="1" /><rect x="9.5" y="2.5" width="3" height="11" rx="1" /></g>
      break
    case 'Missing':
      path = <path fill={c} d="M8 1.5a5.5 5.5 0 0 0-5.5 5.5v7.5l2-1.5 1.7 1.5L8 13l1.8 1.5 1.7-1.5 2 1.5V7A5.5 5.5 0 0 0 8 1.5ZM6 6.2a1 1 0 1 1 0 2 1 1 0 0 1 0-2Zm4 0a1 1 0 1 1 0 2 1 1 0 0 1 0-2Z" />
      break
    default:
      path = (
        <g>
          <circle cx="8" cy="8" r="6.5" fill={c} />
          <text x="8" y="11.5" textAnchor="middle" fontSize="10" fontWeight="700" fill="#fff">?</text>
        </g>
      )
  }
  return (
    <span className="si" title={status}>
      <svg viewBox="0 0 16 16">{path}</svg>
    </span>
  )
}

export function SyncIcon({ status, running }: { status: string; running?: boolean }) {
  const c = syncColor[status] ?? 'var(--unknown)'
  if (running) {
    return (
      <span className="si spin" title="Syncing">
        <svg viewBox="0 0 16 16">
          <path fill="none" stroke="var(--progressing)" strokeWidth="2" strokeLinecap="round" d="M13.5 8A5.5 5.5 0 1 1 8 2.5M13.5 2.5v3h-3" />
        </svg>
      </span>
    )
  }
  return (
    <span className="si" title={status}>
      <svg viewBox="0 0 16 16">
        <circle cx="8" cy="8" r="7" fill={c} />
        {status === 'Synced' && <path d="m4.8 8.2 2.2 2.2 4.2-4.6" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />}
        {status === 'OutOfSync' && <path d="M8 11.5V4.8M5.2 7.4 8 4.6l2.8 2.8" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />}
        {status !== 'Synced' && status !== 'OutOfSync' && <text x="8" y="11.5" textAnchor="middle" fontSize="10" fontWeight="700" fill="#fff">?</text>}
      </svg>
    </span>
  )
}

export function Pill({ kind, status }: { kind: 'health' | 'sync'; status: string }) {
  return (
    <span className="pill">
      {kind === 'health' ? <HealthIcon status={status} /> : <SyncIcon status={status} />}
      {status}
    </span>
  )
}

export function StatBar({ counts, total }: { counts: [string, number][]; total: number }) {
  return (
    <div className="statbar" title={counts.map(([k, n]) => `${k}: ${n}`).join('  ·  ')}>
      {counts.filter(([, n]) => n > 0).map(([k, n]) => (
        <div key={k} style={{ width: `${(n / Math.max(total, 1)) * 100}%`, background: healthColor[k] ?? syncColor[k] ?? k }} />
      ))}
    </div>
  )
}

export function ago(ts?: string) {
  if (!ts) return ''
  const d = (Date.now() - new Date(ts).getTime()) / 1000
  if (isNaN(d)) return ''
  if (d < 60) return 'now'
  if (d < 3600) return `${Math.floor(d / 60)}m`
  if (d < 86400) return `${Math.floor(d / 3600)}h`
  return `${Math.floor(d / 86400)}d`
}
