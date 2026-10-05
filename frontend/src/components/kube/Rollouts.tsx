import { useEffect, useMemo, useState } from 'react'
import * as API from '../../../wailsjs/go/main/App'
import { filterK, getK, useKData, type KObj } from '../../kdata'
import { ConfirmButton, KPage, PhaseIcon, PodsAndLogs, Problem1, useAction, VTable, type Column } from './Common'
import { NoKube } from './Workflows'

type Notify = (m: string, ok: boolean) => void

function Bar({ value, max, color }: { value: number; max: number; color: string }) {
  return (
    <div className="statbar" style={{ width: 70 }}>
      <div style={{ width: `${Math.min(100, (value / Math.max(1, max)) * 100)}%`, background: color }} />
    </div>
  )
}

function stepLabel(o: KObj) {
  if (o.fields.strategy === 'blueGreen') return o.fields.paused ? 'awaiting promotion' : 'blue/green'
  const steps = Number(o.fields.steps ?? 0)
  if (!steps) return '—'
  const idx = Math.min(Number(o.fields.step ?? 0), steps)
  return `step ${idx}/${steps}`
}

export function RolloutsView({ query, ctxFilter, onOpen }: { query: string; ctxFilter: Set<string>; onOpen: (key: string) => void }) {
  const data = useKData()
  const [phase, setPhase] = useState('')
  const all = useMemo(
    () => [...data.objs.values()].filter((o) => o.kind === 'Rollout' && (!ctxFilter.size || ctxFilter.has(o.ctx))),
    [data.version, ctxFilter], // eslint-disable-line react-hooks/exhaustive-deps
  )
  const counts: Record<string, number> = {}
  for (const r of all) counts[r.phase] = (counts[r.phase] ?? 0) + 1
  const list = useMemo(
    () => filterK(all.filter((r) => !phase || r.phase === phase), query).sort((a, b) => b.severity - a.severity || a.name.localeCompare(b.name)),
    [all, phase, query],
  )
  const cols: Column<KObj>[] = [
    { header: '', width: '26px', render: (o) => <PhaseIcon kind="Rollout" phase={o.phase} /> },
    { header: 'Rollout', width: 'minmax(200px,2fr)', render: (o) => <b>{o.name}</b> },
    { header: 'Namespace', width: 'minmax(110px,1fr)', render: (o) => <span className="muted">{o.namespace}<span className="sub">{o.ctx}</span></span> },
    { header: 'Strategy', width: '150px', render: (o) => <span>{String(o.fields.strategy)}<span className="sub">{stepLabel(o)}</span></span> },
    {
      header: 'Weight', width: '110px',
      render: (o) => (Number(o.fields.weight) >= 0 && o.fields.strategy === 'canary'
        ? <span style={{ display: 'flex', gap: 6, alignItems: 'center' }}><Bar value={Number(o.fields.weight)} max={100} color="var(--progressing)" />{String(o.fields.weight)}%</span>
        : <span className="muted">—</span>),
    },
    {
      header: 'Ready', width: '110px',
      render: (o) => <span style={{ display: 'flex', gap: 6, alignItems: 'center' }}><Bar value={Number(o.fields.readyReplicas)} max={Number(o.fields.desired || o.fields.replicas)} color="var(--healthy)" />{String(o.fields.readyReplicas)}/{String(o.fields.desired || o.fields.replicas)}</span>,
    },
    { header: 'Image', width: 'minmax(160px,1.5fr)', render: (o) => <span className="mono muted">{((o.fields.images as string[]) ?? []).map((i) => i.split('/').pop()).join(', ')}</span> },
    { header: 'Problem', width: 'minmax(220px,2.5fr)', render: (o) => <Problem1 o={o} /> },
  ]
  return (
    <>
      <div className="filterbar">
        {['Healthy', 'Progressing', 'Paused', 'Degraded'].map((p) => (
          <span key={p} className={'chip' + (phase === p ? ' on' : '')} onClick={() => setPhase(phase === p ? '' : p)}>
            <PhaseIcon kind="Rollout" phase={p} />{p}<span className="n">{counts[p] ?? 0}</span>
          </span>
        ))}
        <span className="spacer" />
        {(counts.Paused ?? 0) > 0 && <span className="badge warning">{counts.Paused} waiting for promotion</span>}
      </div>
      <VTable rows={list} columns={cols} rowKey={(o) => o.key} onOpen={(o) => onOpen(o.key)} rowClass={(o) => (o.severity === 2 ? 'sev-2' : '')} empty={<NoKube what="rollouts" />} />
    </>
  )
}

type Step = { setWeight?: number; pause?: { duration?: string }; analysis?: unknown; experiment?: unknown; setCanaryScale?: unknown; setHeaderRoute?: unknown }

export function RolloutPage({ objKey, left, onClose, notify, onOpenApp }: { objKey: string; left: number; onClose: () => void; notify: Notify; onOpenApp?: (name: string) => void }) {
  const data = useKData()
  const o = getK(objKey)
  const run = useAction(notify)
  const [raw, setRaw] = useState<Record<string, any> | null>(null) // eslint-disable-line @typescript-eslint/no-explicit-any
  const stamp = o ? `${o.phase}|${o.fields.step}|${o.fields.paused}|${o.fields.aborted}` : ''
  useEffect(() => { API.KObject(objKey).then(setRaw).catch(() => setRaw(null)) }, [objKey, stamp])
  void data.version
  if (!o) return null
  const act = (a: string, label: string) => run(label, () => API.RolloutAction(objKey, a))
  const steps: Step[] = raw?.spec?.strategy?.canary?.steps ?? []
  const idx = Number(o.fields.step ?? 0)
  return (
    <KPage obj={o} left={left} onClose={onClose} icon="ro"
      subtitle={<>
        {String(o.fields.strategy)} · {stepLabel(o)} · revision {String(o.fields.revision || '?')}
        {o.fields.argoApp ? <> · Argo CD app {onOpenApp ? <a onClick={() => onOpenApp(String(o.fields.argoApp))}><b>{String(o.fields.argoApp)}</b></a> : <b>{String(o.fields.argoApp)}</b>}</> : null}
      </>}
      actions={
        <>
          {o.fields.paused && <button className="btn primary" onClick={() => act('promote', 'Promote')}>▶ Promote</button>}
          {o.phase !== 'Healthy' && <ConfirmButton label="⏭ Promote full" confirm="Skip all steps?" onConfirm={() => act('promote-full', 'Promote full')} />}
          {!o.fields.paused && o.phase === 'Progressing' && <button className="btn" onClick={() => act('pause', 'Pause')}>❚❚ Pause</button>}
          {o.fields.aborted
            ? <button className="btn primary" onClick={() => act('retry', 'Retry')}>↻ Retry</button>
            : o.phase !== 'Healthy' && <ConfirmButton className="danger-outline" label="✕ Abort" confirm="Abort and go back to stable?" onConfirm={() => act('abort', 'Abort')} />}
          <ConfirmButton label="↻ Restart pods" confirm="Restart all pods?" onConfirm={() => act('restart', 'Restart')} />
        </>
      }
      tabs={[
        {
          id: 'steps', label: 'Steps',
          render: () => (
            <>
              <div className="kv" style={{ marginBottom: 16 }}>
                <div className="k">Replicas</div><div className="v">{String(o.fields.readyReplicas)} ready / {String(o.fields.updatedReplicas)} updated / {String(o.fields.desired || o.fields.replicas)} desired</div>
                <div className="k">Stable RS</div><div className="v mono">{String(o.fields.stableRS || '—')}</div>
                <div className="k">Canary RS</div><div className="v mono">{o.fields.currentPodHash !== o.fields.stableRS ? String(o.fields.currentPodHash) : '— (fully promoted)'}</div>
                <div className="k">Images</div><div className="v mono">{((o.fields.images as string[]) ?? []).join('\n')}</div>
                {((o.fields.pauseReasons as string[]) ?? []).length > 0 && <><div className="k">Paused by</div><div className="v">{(o.fields.pauseReasons as string[]).join(', ')}</div></>}
              </div>
              {o.fields.strategy === 'canary' && (
                <div className="steps">
                  {steps.map((s, i) => {
                    const state = i < idx ? 'done' : i === idx ? (o.fields.aborted ? 'aborted' : 'current') : 'todo'
                    const label = s.setWeight !== undefined ? `Set weight ${s.setWeight}%` : s.pause ? `Pause${s.pause.duration ? ' ' + s.pause.duration : ' (until promoted)'}` : s.analysis ? 'Analysis' : s.experiment ? 'Experiment' : Object.keys(s)[0]
                    return (
                      <div key={i} className={'step ' + state}>
                        <span className="step-dot">{state === 'done' ? '✓' : i + 1}</span>
                        <span>{label}</span>
                        {state === 'current' && o.fields.paused && <span className="badge warning">waiting</span>}
                      </div>
                    )
                  })}
                  {!steps.length && <div className="muted-sm">No steps defined.</div>}
                </div>
              )}
            </>
          ),
        },
        { id: 'pods', label: 'Pods & logs', flush: true, render: () => <PodsAndLogs objKey={objKey} /> },
      ]}
    />
  )
}
