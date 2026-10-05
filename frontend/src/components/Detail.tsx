import { useEffect, useState } from 'react'
import { argocd, store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'
import { getApp, useData } from '../data'
import { HealthIcon, Pill, SyncIcon, ago } from './Status'
import { ProblemList } from './Problems'
import { ResourceTree } from './ResourceTree'
import { HistoryView } from './History'

type Props = {
  appKey: string
  ctxName: string
  onClose: () => void
  onAction: (action: 'sync' | 'refresh' | 'hard' | 'restart' | 'terminate' | 'delete', keys: string[]) => void
  notify: (msg: string, ok: boolean) => void
  left: number
  onOpenAppSet: (name: string) => void
}

type DetailTab = 'tree' | 'summary' | 'resources' | 'history'

export function Detail({ appKey, ctxName, onClose, onAction, notify, left, onOpenAppSet }: Props) {
  const [tab, setTab] = useState<DetailTab>('tree')
  const [showProblems, setShowProblems] = useState(true)
  const { version } = useData()
  const live = getApp(appKey)
  const [d, setD] = useState<store.AppDetail | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)

  // Reload details when the app changes on the server (debounced by reconcile time).
  const stamp = live ? `${live.reconciledAt}|${live.opPhase}|${live.opFinishedAt}|${live.health}|${live.sync}` : ''
  useEffect(() => {
    let cancel = false
    setLoading(true)
    API.Detail(appKey)
      .then((x) => !cancel && (setD(x), setErr('')))
      .catch((e) => !cancel && setErr(String(e)))
      .finally(() => !cancel && setLoading(false))
    return () => { cancel = true }
  }, [appKey, stamp])

  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [onClose])

  const s = live ?? d?.summary
  void version
  if (!s) return null
  const problems = live?.problems?.length ? live.problems : d?.summary.problems ?? []
  const op = d?.operation
  const failedRes = op?.syncResult?.resources?.filter((r) => r.status === 'SyncFailed' || r.hookPhase === 'Failed' || r.hookPhase === 'Error') ?? []

  const restartOne = async (r: store.ResourceRow) => {
    try {
      await API.RestartResource(appKey, argocd.ResourceAction.createFrom({ Group: r.group, Version: r.version, Kind: r.kind, Namespace: r.namespace, Name: r.name }))
      notify(`Restart sent: ${r.kind}/${r.name}`, true)
    } catch (e) {
      notify(`Failed to restart ${r.kind}/${r.name}: ${e}`, false)
    }
  }

  return (
    <>
      <div className="drawer page" style={{ left }}>
        <header>
          <h2>
            <HealthIcon status={s.health} />
            <span className="selectable">{s.name}</span>
            <button className="btn ghost sm back" onClick={onClose} title="Back to applications (Esc)">← Applications</button>
          </h2>
          <div className="meta">
            {ctxName} · project <b>{s.project}</b>
            {s.appSet && <> · ApplicationSet <a onClick={() => onOpenAppSet(s.appSet)}><b>{s.appSet}</b></a></>} · {s.cluster}/{s.destNamespace}
          </div>
          <div className="tools">
            <Pill kind="health" status={s.health} />
            <span className="pill"><SyncIcon status={s.sync} running={s.opPhase === 'Running'} /> {s.sync}{s.syncRev ? ` · ${s.syncRev}` : ''}</span>
            {s.autoSync && <span className="pill">auto-sync{d?.prune ? ' · prune' : ''}{d?.selfHeal ? ' · self-heal' : ''}</span>}
            <span className="spacer" />
          </div>
          <div className="tools">
            <button className="btn primary" onClick={() => onAction('sync', [appKey])}>⟳ Sync</button>
            <button className="btn" onClick={() => onAction('refresh', [appKey])}>Refresh</button>
            <button className="btn" onClick={() => onAction('hard', [appKey])}>Hard refresh</button>
            <button className="btn" onClick={() => onAction('restart', [appKey])} disabled={!s.workloads}>↻ Restart ({s.workloads})</button>
            {s.opPhase === 'Running' && <button className="btn danger" onClick={() => onAction('terminate', [appKey])}>■ Terminate sync</button>}
            <span className="spacer" />
            <button className="btn ghost" onClick={() => API.OpenInArgo(appKey)}>Open in Argo CD ↗</button>
            <button className="btn danger-outline" onClick={() => onAction('delete', [appKey])}>🗑 Delete</button>
          </div>
          <div className="tabs detail-tabs">
            <div className={'tab' + (tab === 'tree' ? ' active' : '')} onClick={() => setTab('tree')}>Tree</div>
            <div className={'tab' + (tab === 'summary' ? ' active' : '')} onClick={() => setTab('summary')}>Summary</div>
            <div className={'tab' + (tab === 'resources' ? ' active' : '')} onClick={() => setTab('resources')}>
              Resources <span className="badge">{d?.resources?.length ?? 0}</span>
            </div>
            <div className={'tab' + (tab === 'history' ? ' active' : '')} onClick={() => setTab('history')}>History &amp; rollback</div>
          </div>
        </header>
        {problems.length > 0 && (
          <div className="detail-problems selectable">
            <div className="dp-head" onClick={() => setShowProblems(!showProblems)}>
              <b>Why it is failing</b> <span className={'badge ' + (problems.some((p) => p.severity === 'error') ? 'error' : 'warning')}>{problems.length}</span>
              {!showProblems && <span className="dp-first">{(problems[0].resource ? problems[0].resource + ': ' : '') + problems[0].message}</span>}
              <span className="spacer" />
              <span className="muted-sm">{showProblems ? 'hide' : 'show'}</span>
            </div>
            {showProblems && <ProblemList problems={problems} />}
          </div>
        )}
        {err && <div className="alert" style={{ margin: '12px 20px 0' }}>Could not load details: {err}</div>}
        {tab === 'tree' && (
          <div className="body tree-body">
            {d ? (
              <ResourceTree
                appKey={appKey}
                app={s}
                nodes={d.tree ?? []}
                onRestart={(n) => restartOne(store.ResourceRow.createFrom({ group: n.group, version: n.version, kind: n.kind, namespace: n.namespace, name: n.name }))}
              />
            ) : (
              <div className="empty">{loading ? 'Loading resource tree…' : 'No data'}</div>
            )}
            {d?.treeError && <div className="alert" style={{ margin: 12 }}>resource-tree: {d.treeError}</div>}
          </div>
        )}
        {tab !== 'tree' && (
        <div className="body selectable">

          {tab === 'history' && <HistoryView appKey={appKey} autoSync={s.autoSync} notify={notify} />}

          {tab === 'summary' && op && (
            <section>
              <h4>Last operation</h4>
              <div className="kv">
                <div className="k">Phase</div>
                <div className="v">
                  <b style={{ color: op.phase === 'Succeeded' ? 'var(--healthy)' : op.phase === 'Running' ? 'var(--progressing)' : 'var(--error)' }}>{op.phase}</b>
                  {op.retryCount ? ` · attempt ${op.retryCount}` : ''}
                </div>
                <div className="k">Message</div>
                <div className="v msg">{op.message || '—'}</div>
                <div className="k">Started / finished</div>
                <div className="v">{op.startedAt || '—'} → {op.finishedAt || '…'}</div>
                {op.syncResult?.revision && (<><div className="k">Revision</div><div className="v mono">{op.syncResult.revision}</div></>)}
              </div>
              {failedRes.length > 0 && (
                <table className="mini-table" style={{ marginTop: 10 }}>
                  <thead><tr><th>Failed resource</th><th>Message</th></tr></thead>
                  <tbody>
                    {failedRes.map((r, i) => (
                      <tr key={i} className="bad">
                        <td className="mono">{r.kind}/{r.name}{r.hookType ? ` (hook ${r.hookType})` : ''}</td>
                        <td className="msg">{r.message}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </section>
          )}

          {tab === 'summary' && <section>
            <h4>Source and destination</h4>
            <div className="kv">
              {(d?.sources ?? []).map((src, i) => (
                <Source key={i} src={src} />
              ))}
              <div className="k">Cluster</div>
              <div className="v">{s.cluster} <span style={{ color: 'var(--fg-faint)' }}>{s.clusterServer}</span></div>
              <div className="k">Namespace</div>
              <div className="v">{s.destNamespace || '—'}</div>
              <div className="k">Reconciled</div>
              <div className="v">{s.reconciledAt ? `${ago(s.reconciledAt)} ago` : '—'}</div>
            </div>
          </section>}

          {tab === 'resources' && <section>
            <h4>Resources {loading && <span style={{ textTransform: 'none' }}>· loading…</span>}</h4>
            {d?.treeError && <div className="alert" style={{ marginBottom: 8 }}>resource-tree: {d.treeError}</div>}
            <ResourceTable rows={d?.resources ?? []} onRestart={restartOne} />
          </section>}

          {tab === 'resources' && (d?.pods?.length ?? 0) > 0 && (
            <section>
              <h4>Pods</h4>
              <table className="mini-table">
                <thead><tr><th>Pod</th><th>Health</th><th>Status</th><th>Owner</th></tr></thead>
                <tbody>
                  {d!.pods.map((r) => (
                    <tr key={r.namespace + r.name} className={r.health === 'Degraded' ? 'bad' : ''}>
                      <td className="mono">{r.name}</td>
                      <td><HealthIcon status={r.health || 'Unknown'} /> {r.health}</td>
                      <td className="msg">{r.message}</td>
                      <td className="msg">{r.parent}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          )}

          {tab === 'summary' && (d?.conditions?.length ?? 0) > 0 && (
            <section>
              <h4>Conditions</h4>
              <table className="mini-table">
                <tbody>
                  {d!.conditions.map((c, i) => (
                    <tr key={i} className={c.type.endsWith('Error') ? 'bad' : ''}>
                      <td style={{ width: 200 }}><b>{c.type}</b><div style={{ color: 'var(--fg-faint)' }}>{c.lastTransitionTime}</div></td>
                      <td className="msg">{c.message}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          )}

          {tab === 'summary' && (d?.history?.length ?? 0) > 0 && (
            <section>
              <h4>Deploy history</h4>
              <table className="mini-table">
                <thead><tr><th>#</th><th>Revision</th><th>Deployed</th></tr></thead>
                <tbody>
                  {d!.history.slice(0, 15).map((h) => (
                    <tr key={h.id}>
                      <td>{h.id}</td>
                      <td className="mono">{(h.revision || h.revisions?.[0] || '').slice(0, 12)}</td>
                      <td>{h.deployedAt} <span style={{ color: 'var(--fg-faint)' }}>({ago(h.deployedAt)})</span></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          )}
        </div>
        )}
      </div>
    </>
  )
}

function Source({ src }: { src: argocd.AppSource }) {
  return (
    <>
      <div className="k">Repository</div>
      <div className="v">{src.repoURL}</div>
      <div className="k">{src.chart ? 'Chart' : 'Path'}</div>
      <div className="v mono">{src.chart || src.path || '—'}</div>
      <div className="k">Target revision</div>
      <div className="v mono">{src.targetRevision || 'HEAD'}</div>
    </>
  )
}

function ResourceTable({ rows, onRestart }: { rows: store.ResourceRow[]; onRestart: (r: store.ResourceRow) => void }) {
  const [onlyBad, setOnlyBad] = useState(false)
  const bad = (r: store.ResourceRow) => r.health === 'Degraded' || r.health === 'Missing' || r.sync === 'OutOfSync'
  const shown = onlyBad ? rows.filter(bad) : [...rows].sort((a, b) => Number(bad(b)) - Number(bad(a)))
  return (
    <>
      <label className="check" style={{ marginBottom: 6 }}>
        <input type="checkbox" checked={onlyBad} onChange={(e) => setOnlyBad(e.target.checked)} />
        only with problems ({rows.filter(bad).length} of {rows.length})
      </label>
      <table className="mini-table">
        <thead><tr><th>Kind</th><th>Name</th><th>Sync</th><th>Health</th><th>Message</th><th /></tr></thead>
        <tbody>
          {shown.map((r) => (
            <tr key={r.group + r.kind + r.namespace + r.name} className={r.health === 'Degraded' ? 'bad' : ''}>
              <td>{r.kind}</td>
              <td className="mono">{r.namespace ? `${r.namespace}/` : ''}{r.name}{r.prune ? ' (prune)' : ''}</td>
              <td><SyncIcon status={r.sync || 'Unknown'} /></td>
              <td>{r.health ? <HealthIcon status={r.health} /> : '—'}</td>
              <td className="msg">{r.message}</td>
              <td>{r.restartable && <button className="btn sm" onClick={() => onRestart(r)}>restart</button>}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
