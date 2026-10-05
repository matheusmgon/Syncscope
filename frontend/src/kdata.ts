// Mirror of the Kubernetes-backed Argo objects (Workflows, Events, Rollouts)
// streamed by the backend, same pattern as data.ts.
import { useSyncExternalStore } from 'react'
import { kubestore } from '../wailsjs/go/models'
import * as API from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

export type KObj = kubestore.Obj & { hay: string }

type State = {
  version: number
  objs: Map<string, KObj>
  statuses: kubestore.ContextStatus[]
}

const state: State = { version: 0, objs: new Map(), statuses: [] }
const listeners = new Set<() => void>()
let snap = { ...state }
function bump() {
  state.version++
  snap = { ...state }
  listeners.forEach((l) => l())
}

function index(o: kubestore.Obj): KObj {
  const f = o.fields ?? {}
  const parts = [o.name, o.namespace, o.kind, o.ctx, o.phase, String(f.template ?? ''), String(f.cron ?? ''), String(f.argoApp ?? ''),
    String(f.strategy ?? ''), ...(Array.isArray(f.images) ? f.images : [])]
  if (o.labels) for (const k in o.labels) parts.push(k + '=' + o.labels[k])
  return Object.assign(o, { hay: parts.join('\u0001').toLowerCase() }) as KObj
}

export function useKData() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => snap,
  )
}

export function getK(key: string) {
  return state.objs.get(key)
}

let started = false
export async function startKData() {
  if (started) return
  started = true
  EventsOn('k:snapshot', (p: { ctx: string; kind: string; items: kubestore.Obj[] | null }) => {
    const prefix = `${p.ctx}|${p.kind}|`
    for (const k of state.objs.keys()) if (k.startsWith(prefix)) state.objs.delete(k)
    for (const o of p.items ?? []) state.objs.set(o.key, index(o))
    bump()
  })
  EventsOn('k:delta', (p: { upserts: kubestore.Obj[]; deletes: string[] }) => {
    for (const k of p.deletes ?? []) state.objs.delete(k)
    for (const o of p.upserts ?? []) state.objs.set(o.key, index(o))
    bump()
  })
  EventsOn('k:reset', (p: { ctx: string }) => {
    for (const k of state.objs.keys()) if (k.startsWith(p.ctx + '|')) state.objs.delete(k)
    bump()
  })
  EventsOn('k:status', (s: kubestore.ContextStatus[]) => {
    state.statuses = s ?? []
    bump()
  })
  const [objs, st] = await Promise.all([API.KObjects(), API.KubeStatuses()])
  for (const o of objs ?? []) if (!state.objs.has(o.key)) state.objs.set(o.key, index(o))
  state.statuses = st ?? []
  bump()
}

// Text search over kube objects: free text + field:value (ns:, ctx:, phase:, kind:, template:) + is:error/warning/ok.
export function filterK(list: KObj[], q: string): KObj[] {
  const terms = [...q.matchAll(/(-?)(?:([a-zA-Z]+):)?(?:"([^"]*)"|(\S+))/g)].map((m) => ({
    neg: m[1] === '-', field: (m[2] ?? '').toLowerCase(), value: (m[3] ?? m[4] ?? '').toLowerCase(),
  }))
  if (!terms.length) return list
  return list.filter((o) =>
    terms.every((t) => {
      let ok: boolean
      switch (t.field) {
        case '': ok = o.hay.includes(t.value); break
        case 'ns': case 'namespace': ok = o.namespace.toLowerCase().includes(t.value); break
        case 'ctx': case 'cluster': ok = o.ctx.toLowerCase().includes(t.value); break
        case 'phase': case 'status': ok = o.phase.toLowerCase().startsWith(t.value); break
        case 'kind': ok = o.kind.toLowerCase() === t.value; break
        case 'name': ok = o.name.toLowerCase().includes(t.value); break
        case 'template': ok = String(o.fields?.template ?? '').toLowerCase().includes(t.value); break
        case 'cron': ok = String(o.fields?.cron ?? '').toLowerCase().includes(t.value); break
        case 'app': ok = String(o.fields?.argoApp ?? '').toLowerCase().includes(t.value); break
        case 'is':
          ok = t.value === 'error' ? o.severity === 2 : t.value === 'warning' ? o.severity === 1 : t.value === 'problem' ? o.severity > 0 : t.value === 'ok' ? o.severity === 0 : false
          break
        default: ok = o.hay.includes(t.field + ':' + t.value)
      }
      return ok !== t.neg
    }),
  )
}
