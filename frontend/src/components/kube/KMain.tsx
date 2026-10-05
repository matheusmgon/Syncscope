import { useDeferredValue, useEffect, useRef } from 'react'
import { useKData } from '../../kdata'
import { WorkflowsView, WorkflowPage, TemplatePage, CronPage } from './Workflows'
import { RolloutsView, RolloutPage } from './Rollouts'
import { EventsView, EventObjPage } from './Events'
import { SavedSearches } from '../SavedSearches'

export type Product = 'cd' | 'workflows' | 'rollouts' | 'events'

export const productKinds: Record<Exclude<Product, 'cd'>, string[]> = {
  workflows: ['Workflow', 'WorkflowTemplate', 'CronWorkflow'],
  rollouts: ['Rollout'],
  events: ['EventSource', 'Sensor', 'EventBus'],
}

const placeholder: Record<string, string> = {
  workflows: 'Search workflows…  e.g. etl  template:ml-train  phase:failed  is:error  ns:argo  cluster:prod',
  rollouts: 'Search rollouts…  e.g. checkout  phase:paused  is:error  app:checkout-api  ns:payments',
  events: 'Search event sources and sensors…  e.g. kafka  is:error  ns:argo-events',
}

const title: Record<string, string> = { workflows: 'Argo Workflows', rollouts: 'Argo Rollouts', events: 'Argo Events' }

// Main area for the Kubernetes-backed products.
export function KMain({ product, query, setQuery, ctxFilter, onOpen }: {
  product: Exclude<Product, 'cd'>
  query: string
  setQuery: (q: string) => void
  ctxFilter: Set<string>
  onOpen: (key: string) => void
}) {
  const data = useKData()
  const dq = useDeferredValue(query)
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.key === 'k' && (e.metaKey || e.ctrlKey)) || (e.key === '/' && !(e.target as HTMLElement)?.closest('input, textarea, select'))) {
        e.preventDefault()
        ref.current?.focus()
        ref.current?.select()
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [])
  const kinds = productKinds[product]
  const missing = data.statuses.filter((s) => s.state === 'ok' && kinds.every((k) => s.kinds?.[k]?.state === 'missing'))
  const forbidden = data.statuses.filter((s) => kinds.some((k) => s.kinds?.[k]?.state === 'forbidden'))
  const down = data.statuses.filter((s) => s.state === 'error')
  const failing = [...data.objs.values()].filter((o) => kinds.includes(o.kind) && o.severity === 2 && (!ctxFilter.size || ctxFilter.has(o.ctx)))
  return (
    <>
      <div className="topbar">
        <div className="topbar-row">
          <div className="search">
            <span style={{ color: 'var(--fg-faint)' }}>⌕</span>
            <input ref={ref} value={query} onChange={(e) => setQuery(e.target.value)} placeholder={placeholder[product]} spellCheck={false} />
            {query && <button className="x" style={{ fontSize: 14 }} onClick={() => setQuery('')}>✕</button>}
            <span className="kbd">⌘K</span>
          </div>
          <SavedSearches scope={product} query={query} setQuery={setQuery} />
        </div>
        <div className="tabs"><div className="tab active">{title[product]}</div></div>
      </div>
      {down.map((s) => <div key={s.name} className="banner" style={{ cursor: 'default' }}>⚠ {s.name}: {s.message}</div>)}
      {failing.length > 0 && (
        <div className="banner" onClick={() => setQuery(query.includes('is:error') ? query : (query + ' is:error').trim())}>
          <span>⚠ {failing.length} failing</span>
          <span className="examples">{failing.slice(0, 3).map((o) => `${o.name}: ${o.problems?.[0]?.message ?? ''}`).join('  ·  ')}</span>
          <span>Show only failing →</span>
        </div>
      )}
      {missing.length > 0 && <div className="banner info">{title[product]} is not installed in: {missing.map((s) => s.name).join(', ')}</div>}
      {forbidden.length > 0 && <div className="banner info">No permission to list {title[product]} objects in: {forbidden.map((s) => s.name).join(', ')}</div>}
      {product === 'workflows' && <WorkflowsView query={dq} ctxFilter={ctxFilter} onOpen={onOpen} />}
      {product === 'rollouts' && <RolloutsView query={dq} ctxFilter={ctxFilter} onOpen={onOpen} />}
      {product === 'events' && <EventsView query={dq} ctxFilter={ctxFilter} onOpen={onOpen} />}
    </>
  )
}

// Detail page router by kind (key = ctx|Kind|ns/name).
export function KPageRouter({ objKey, left, onClose, notify, onOpenKey, onOpenApp }: {
  objKey: string
  left: number
  onClose: () => void
  notify: (m: string, ok: boolean) => void
  onOpenKey: (k: string) => void
  onOpenApp: (name: string) => void
}) {
  const kind = objKey.split('|')[1]
  const props = { objKey, left, onClose, notify }
  switch (kind) {
    case 'Workflow': return <WorkflowPage key={objKey} {...props} onOpenKey={onOpenKey} />
    case 'WorkflowTemplate': return <TemplatePage key={objKey} {...props} onOpenKey={onOpenKey} />
    case 'CronWorkflow': return <CronPage key={objKey} {...props} onOpenKey={onOpenKey} />
    case 'Rollout': return <RolloutPage key={objKey} {...props} onOpenApp={onOpenApp} />
    default: return <EventObjPage key={objKey} {...props} />
  }
}
