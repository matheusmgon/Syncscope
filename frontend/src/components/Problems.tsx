import { useMemo, useState } from 'react'
import { store } from '../../wailsjs/go/models'
import type { App } from '../data'
import { HealthIcon, SyncIcon, ago } from './Status'

const sourceLabel: Record<string, string> = {
  sync: 'sync', condition: 'condition', health: 'health', cluster: 'cluster', appset: 'appset', retry: 'retry',
}

export function ProblemList({ problems }: { problems: store.Problem[] }) {
  const hasErr = problems.some((p) => p.severity === 'error')
  return (
    <div className={'problem-box' + (hasErr ? '' : ' warn')}>
      {problems.map((p, i) => (
        <div key={i} className={'problem-line ' + p.severity}>
          <span className={'tag ' + p.severity}>{sourceLabel[p.source] ?? p.source}</span>
          <div>
            {p.resource && <div className="res">{p.resource}</div>}
            <div className="msg">{p.message}</div>
          </div>
        </div>
      ))}
    </div>
  )
}

// Collapse app-specific bits so identical failures across apps group together.
function normalize(msg: string) {
  return msg
    .replace(/\b[0-9a-f]{7,40}\b/g, '<sha>')
    .replace(/\b[a-z0-9]+(-[a-z0-9]+)*-[a-z0-9]{5,10}-[a-z0-9]{5}\b/g, '<pod>')
    .replace(/\d+/g, '#')
    .slice(0, 240)
}

type Props = {
  apps: App[]
  appsets: store.AppSetSummary[]
  clusters: store.ClusterSummary[]
  ctxNames: Map<string, string>
  onOpen: (key: string) => void
  onAction: (action: 'sync' | 'refresh' | 'hard' | 'restart' | 'terminate', keys: string[]) => void
  onFilterAppSet: (name: string) => void
}

export function ProblemsView(p: Props) {
  const [mode, setMode] = useState<'app' | 'cause'>('cause')
  const [showWarn, setShowWarn] = useState(false)
  const [open, setOpen] = useState<Set<string>>(new Set())

  const withProblems = useMemo(
    () => p.apps.filter((a) => a.severity === 2 || (showWarn && a.severity === 1)).sort((a, b) => b.severity - a.severity || a.name.localeCompare(b.name)),
    [p.apps, showWarn],
  )

  const causes = useMemo(() => {
    const m = new Map<string, { msg: string; source: string; severity: string; apps: App[] }>()
    for (const a of withProblems) {
      const seen = new Set<string>()
      for (const pr of a.problems ?? []) {
        if (pr.severity !== 'error' && !showWarn) continue
        const k = pr.source + '|' + normalize(pr.message)
        if (seen.has(k)) continue
        seen.add(k)
        let c = m.get(k)
        if (!c) m.set(k, (c = { msg: pr.message, source: pr.source, severity: pr.severity, apps: [] }))
        c.apps.push(a)
      }
    }
    return [...m.values()].sort((a, b) => (a.severity === b.severity ? b.apps.length - a.apps.length : a.severity === 'error' ? -1 : 1))
  }, [withProblems, showWarn])

  const badSets = p.appsets.filter((s) => s.problems?.length)
  const badClusters = p.clusters.filter((c) => c.state === 'Failed')
  const toggle = (k: string) => {
    const s = new Set(open)
    s.has(k) ? s.delete(k) : s.add(k)
    setOpen(s)
  }

  return (
    <div className="problems-view selectable">
      <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
        <div className="seg">
          <button className={mode === 'cause' ? 'on' : ''} onClick={() => setMode('cause')}>Group by cause</button>
          <button className={mode === 'app' ? 'on' : ''} onClick={() => setMode('app')}>By application</button>
        </div>
        <label className="check"><input type="checkbox" checked={showWarn} onChange={(e) => setShowWarn(e.target.checked)} /> include warnings</label>
        <span className="spacer" />
        <span style={{ color: 'var(--fg-muted)' }}>{withProblems.length} apps · {causes.length} distinct causes</span>
      </div>

      {badClusters.length > 0 && (
        <>
          <div className="group-title">Unreachable clusters</div>
          {badClusters.map((c) => (
            <div key={c.ctx + c.server} className="pcard">
              <div className="phead">
                <span className="dot error" />
                <span className="name">{c.name || c.server}</span>
                <span className="where">{p.ctxNames.get(c.ctx)} · {c.server}</span>
              </div>
              <div className="plist"><ProblemList problems={[store.Problem.createFrom({ severity: 'error', source: 'cluster', message: c.message || 'Failed' })]} /></div>
            </div>
          ))}
        </>
      )}

      {badSets.length > 0 && (
        <>
          <div className="group-title">Failing ApplicationSets</div>
          {badSets.map((s) => (
            <div key={s.key} className="pcard">
              <div className="phead" onClick={() => p.onFilterAppSet(s.key)}>
                <span className="name">{s.name}</span>
                <span className="where">{p.ctxNames.get(s.ctx)} · {s.namespace}</span>
              </div>
              <div className="plist"><ProblemList problems={s.problems ?? []} /></div>
            </div>
          ))}
        </>
      )}

      {withProblems.length === 0 && badSets.length === 0 && badClusters.length === 0 && (
        <div className="empty">
          <h3>No problems 🎉</h3>
          All applications are error-free{showWarn ? ' and warning-free' : ''}.
        </div>
      )}

      {mode === 'cause' && causes.length > 0 && <div className="group-title">Causes</div>}
      {mode === 'cause' &&
        causes.map((c, i) => {
          const k = 'c' + i + c.msg
          const isOpen = open.has(k)
          const keys = c.apps.map((a) => a.key)
          return (
            <div key={k} className={'pcard' + (c.severity === 'error' ? '' : ' warn')}>
              <div className="phead" onClick={() => toggle(k)}>
                <span className={'badge ' + (c.severity === 'error' ? 'error' : 'warning')}>{c.apps.length}</span>
                <span className="msg" style={{ flex: 1, color: c.severity === 'error' ? 'var(--error-fg)' : 'var(--warning-fg)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                  <b>[{sourceLabel[c.source] ?? c.source}]</b> {c.msg}
                </span>
                <span className="pactions" onClick={(e) => e.stopPropagation()}>
                  <button className="btn sm" onClick={() => p.onAction('sync', keys)}>Sync {c.apps.length}</button>
                  <button className="btn sm" onClick={() => p.onAction('hard', keys)}>Hard refresh</button>
                  <button className="btn sm" onClick={() => p.onAction('restart', keys)}>Restart</button>
                </span>
              </div>
              <div className="plist">
                {(isOpen ? c.apps : c.apps.slice(0, 8)).map((a) => (
                  <span key={a.key} className="pill" style={{ margin: '0 6px 6px 0', cursor: 'pointer' }} onClick={() => p.onOpen(a.key)}>
                    <HealthIcon status={a.health} /> {a.name}
                    {p.ctxNames.size > 1 && <span style={{ color: 'var(--fg-faint)' }}>{p.ctxNames.get(a.ctx)}</span>}
                  </span>
                ))}
                {!isOpen && c.apps.length > 8 && <a onClick={() => toggle(k)}>+ {c.apps.length - 8} apps</a>}
              </div>
            </div>
          )
        })}

      {mode === 'app' && withProblems.length > 0 && <div className="group-title">Applications</div>}
      {mode === 'app' &&
        withProblems.slice(0, 500).map((a) => (
          <div key={a.key} className={'pcard' + (a.severity === 2 ? '' : ' warn')}>
            <div className="phead" onClick={() => p.onOpen(a.key)}>
              <HealthIcon status={a.health} />
              <SyncIcon status={a.sync} running={a.opPhase === 'Running'} />
              <span className="name">{a.name}</span>
              <span className="where">
                {p.ctxNames.get(a.ctx)} · {a.appSet ? `${a.appSet} · ` : ''}{a.cluster}/{a.destNamespace}
                {a.opFinishedAt ? ` · last sync ${ago(a.opFinishedAt)} ago` : ''}
              </span>
              <span className="pactions" onClick={(e) => e.stopPropagation()}>
                <button className="btn sm" onClick={() => p.onAction('sync', [a.key])}>Sync</button>
                <button className="btn sm" onClick={() => p.onAction('restart', [a.key])} disabled={!a.workloads}>Restart</button>
              </span>
            </div>
            <div className="plist"><ProblemList problems={a.problems ?? []} /></div>
          </div>
        ))}
      {mode === 'app' && withProblems.length > 500 && (
        <div className="empty">Showing 500 of {withProblems.length}. Use “Group by cause” or search with <code>is:error</code>.</div>
      )}
    </div>
  )
}
