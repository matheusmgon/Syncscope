import { describe, expect, it } from 'vitest'
import type { App } from './data'
import type { store } from '../wailsjs/go/models'
import { filterApps, matchText, parseQuery, problemText, sortApps } from './search'

// Same "hay" construction as index() in data.ts (kept in sync by hand: data.ts
// imports the Wails runtime, which cannot load under Node).
function mkApp(p: Partial<store.AppSummary> & { name: string }): App {
  const a = {
    key: (p.ctx ?? 'c1') + '|argocd/' + p.name,
    ctx: 'c1',
    appNamespace: 'argocd',
    project: 'default',
    appSet: '',
    cluster: 'in-cluster',
    clusterServer: 'https://kubernetes.default.svc',
    destNamespace: 'default',
    repo: '',
    path: '',
    targetRev: 'HEAD',
    syncRev: '',
    sync: 'Synced',
    health: 'Healthy',
    autoSync: false,
    deleting: false,
    severity: 0,
    workloads: 0,
    ...p,
  } as store.AppSummary
  const parts = [a.name, a.appSet, a.project, a.cluster, a.clusterServer, a.destNamespace, a.repo, a.path, a.targetRev, a.appNamespace]
  if (a.labels) for (const k in a.labels) parts.push(k + '=' + a.labels[k])
  return Object.assign(a, { hay: parts.join('\u0001').toLowerCase() }) as App
}

const apps: App[] = [
  mkApp({
    name: 'payments-api',
    appSet: 'payments',
    project: 'core',
    cluster: 'prod-eu',
    clusterServer: 'https://prod-eu.example.com',
    destNamespace: 'payments',
    repo: 'https://github.com/acme/deploy',
    path: 'apps/payments',
    targetRev: 'main',
    syncRev: 'abc123',
    autoSync: true,
    labels: { team: 'core', tier: 'backend' },
    opFinishedAt: '2026-01-02T00:00:00Z',
  }),
  mkApp({
    name: 'payments-api-legacy',
    project: 'core',
    cluster: 'prod-us',
    clusterServer: 'https://prod-us.internal',
    destNamespace: 'payments',
    repo: 'https://github.com/acme/deploy',
    path: 'apps/payments-legacy',
    targetRev: 'main',
    syncRev: 'def456',
    sync: 'OutOfSync',
    health: 'Degraded',
    severity: 2,
    opPhase: 'Running',
    labels: { team: 'core' },
    problems: [{ severity: 'error', source: 'health', resource: 'Pod/payments-legacy-0', message: 'OOMKilled' }] as store.Problem[],
    opFinishedAt: '2026-01-03T00:00:00Z',
  }),
  mkApp({
    name: 'web',
    ctx: 'c2',
    appSet: 'frontend',
    project: 'web',
    cluster: 'staging',
    clusterServer: 'https://10.0.0.1',
    destNamespace: 'web',
    repo: 'https://github.com/acme/web',
    path: 'chart',
    targetRev: 'v2',
    syncRev: 'fff000',
    health: 'Progressing',
    severity: 1,
    autoSync: true,
    deleting: true,
    labels: { team: 'web-platform' },
    opFinishedAt: '2026-01-01T00:00:00Z',
  }),
]

const ctxNames = new Map([
  ['c1', 'Production'],
  ['c2', 'Staging'],
])

const names = (q: string, pred?: (a: App) => boolean, extra?: (a: App) => string) =>
  filterApps(apps, q, ctxNames, pred, extra).map((a) => a.name)

describe('parseQuery', () => {
  it('parses free text', () => {
    expect(parseQuery('payments')).toEqual([{ neg: false, field: '', value: 'payments', exact: false }])
  })

  it('lowercases fields and values', () => {
    expect(parseQuery('Name:API')).toEqual([{ neg: false, field: 'name', value: 'api', exact: false }])
  })

  it('parses negation', () => {
    expect(parseQuery('-legacy -health:degraded')).toEqual([
      { neg: true, field: '', value: 'legacy', exact: false },
      { neg: true, field: 'health', value: 'degraded', exact: false },
    ])
  })

  it('parses quoted phrases as exact', () => {
    expect(parseQuery('"Hello World" name:"payments-api"')).toEqual([
      { neg: false, field: '', value: 'hello world', exact: true },
      { neg: false, field: 'name', value: 'payments-api', exact: true },
    ])
  })

  it('resolves field aliases', () => {
    const fields = parseQuery('as:a set:b proj:c p:d c:e dest:f namespace:g n:h h:i s:j l:k context:m').map((t) => t.field)
    expect(fields).toEqual(['appset', 'appset', 'project', 'project', 'cluster', 'cluster', 'ns', 'name', 'health', 'sync', 'label', 'ctx'])
  })

  it('ignores blank input', () => {
    expect(parseQuery('')).toEqual([])
    expect(parseQuery('   ')).toEqual([])
  })
})

describe('filterApps', () => {
  it('returns everything for an empty query', () => {
    expect(names('')).toEqual(['payments-api', 'payments-api-legacy', 'web'])
  })

  it('matches free text over the hay and ANDs terms', () => {
    expect(names('payments')).toEqual(['payments-api', 'payments-api-legacy'])
    expect(names('payments core')).toEqual(['payments-api', 'payments-api-legacy'])
    expect(names('PAYMENTS staging')).toEqual([])
    expect(names('team=web')).toEqual(['web']) // labels are part of the hay
  })

  it('supports negation', () => {
    expect(names('payments -legacy')).toEqual(['payments-api'])
    expect(names('-payments')).toEqual(['web'])
    expect(names('-is:error')).toEqual(['payments-api', 'web'])
  })

  it('treats quoted field values as exact matches', () => {
    expect(names('name:payments-api')).toEqual(['payments-api', 'payments-api-legacy'])
    expect(names('name:"payments-api"')).toEqual(['payments-api'])
    expect(names('name:"PAYMENTS-API"')).toEqual(['payments-api'])
    expect(names('name:"payments"')).toEqual([])
    expect(names('-name:"payments-api"')).toEqual(['payments-api-legacy', 'web'])
  })

  it('matches quoted free text as a phrase', () => {
    expect(names('"payments-api-legacy"')).toEqual(['payments-api-legacy'])
  })

  it('filters by is:', () => {
    expect(names('is:error')).toEqual(['payments-api-legacy'])
    expect(names('is:failed')).toEqual(['payments-api-legacy'])
    expect(names('is:warning')).toEqual(['web'])
    expect(names('is:problem')).toEqual(['payments-api-legacy', 'web'])
    expect(names('is:ok')).toEqual(['payments-api'])
    expect(names('is:auto')).toEqual(['payments-api', 'web'])
    expect(names('is:manual')).toEqual(['payments-api-legacy'])
    expect(names('is:running')).toEqual(['payments-api-legacy'])
    expect(names('is:deleting')).toEqual(['web'])
    expect(names('is:outofsync')).toEqual(['payments-api-legacy'])
    expect(names('is:nonsense')).toEqual([])
  })

  it('filters by label:k and label:k=v', () => {
    expect(names('label:team')).toEqual(['payments-api', 'payments-api-legacy', 'web'])
    expect(names('label:tier')).toEqual(['payments-api'])
    expect(names('label:team=core')).toEqual(['payments-api', 'payments-api-legacy'])
    expect(names('label:Team=WEB')).toEqual(['web']) // value is a substring, case-insensitive
    expect(names('label:tier=frontend')).toEqual([])
    expect(names('label:missing')).toEqual([])
    expect(names('-label:tier')).toEqual(['payments-api-legacy', 'web'])
  })

  it('filters by health and sync prefixes', () => {
    expect(names('health:deg')).toEqual(['payments-api-legacy'])
    expect(names('h:healthy')).toEqual(['payments-api'])
    expect(names('sync:out')).toEqual(['payments-api-legacy'])
    expect(names('s:synced')).toEqual(['payments-api', 'web'])
  })

  it('filters by appset, including appset:none', () => {
    expect(names('appset:pay')).toEqual(['payments-api'])
    expect(names('as:frontend')).toEqual(['web'])
    expect(names('appset:none')).toEqual(['payments-api-legacy'])
  })

  it('filters by project, cluster, namespace, repo, rev and context', () => {
    expect(names('project:web')).toEqual(['web'])
    expect(names('cluster:prod')).toEqual(['payments-api', 'payments-api-legacy'])
    expect(names('cluster:example.com')).toEqual(['payments-api']) // server URL
    expect(names('ns:payments')).toEqual(['payments-api', 'payments-api-legacy'])
    expect(names('repo:acme/web')).toEqual(['web'])
    expect(names('repo:payments-legacy')).toEqual(['payments-api-legacy']) // path
    expect(names('rev:abc')).toEqual(['payments-api'])
    expect(names('rev:v2')).toEqual(['web'])
    expect(names('ctx:staging')).toEqual(['web'])
    expect(names('ctx:prod')).toEqual(['payments-api', 'payments-api-legacy'])
  })

  it('treats unknown fields as free text', () => {
    expect(names('foo:bar')).toEqual([])
  })

  it('applies the predicate and extra haystack', () => {
    expect(names('', (a) => a.ctx === 'c2')).toEqual(['web'])
    expect(names('payments', (a) => a.autoSync)).toEqual(['payments-api'])
    expect(names('oomkilled')).toEqual([])
    expect(names('oomkilled', undefined, problemText)).toEqual(['payments-api-legacy'])
  })
})

describe('matchText', () => {
  it('matches every term case-insensitively', () => {
    expect(matchText('', 'anything')).toBe(true)
    expect(matchText('guest', 'guestbook-set', 'argocd')).toBe(true)
    expect(matchText('GUEST argo', 'guestbook-set', 'argocd')).toBe(true)
    expect(matchText('guest prod', 'guestbook-set', 'argocd')).toBe(false)
  })

  it('supports negation', () => {
    expect(matchText('-set', 'guestbook-set')).toBe(false)
    expect(matchText('guest -prod', 'guestbook-set', 'staging')).toBe(true)
  })

  it('supports quoted phrases', () => {
    expect(matchText('"connection refused"', 'dial tcp: Connection refused')).toBe(true)
    expect(matchText('"refused connection"', 'dial tcp: Connection refused')).toBe(false)
  })

  it('ignores undefined fields and does not match across field boundaries', () => {
    expect(matchText('x', undefined, 'x')).toBe(true)
    expect(matchText('ab', 'a', 'b')).toBe(false)
  })

  it('matches the value of field:value terms', () => {
    expect(matchText('cluster:prod', 'prod-eu')).toBe(true)
    expect(matchText('-cluster:prod', 'prod-eu')).toBe(false)
  })
})

describe('sortApps', () => {
  const sorted = (key: Parameters<typeof sortApps>[1], desc = false) => sortApps([...apps], key, desc).map((a) => a.name)

  it('sorts by severity (worst first), then name', () => {
    expect(sorted('severity')).toEqual(['payments-api-legacy', 'web', 'payments-api'])
  })

  it('sorts by health rank', () => {
    expect(sorted('health')).toEqual(['payments-api-legacy', 'web', 'payments-api'])
  })

  it('sorts by name and reverses with desc', () => {
    expect(sorted('name')).toEqual(['payments-api', 'payments-api-legacy', 'web'])
    expect(sorted('name', true)).toEqual(['web', 'payments-api-legacy', 'payments-api'])
  })

  it('sorts by last sync, most recent first', () => {
    expect(sorted('synced')).toEqual(['payments-api-legacy', 'payments-api', 'web'])
  })
})
