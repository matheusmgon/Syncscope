import { useEffect, useState } from 'react'
import { store } from '../../wailsjs/go/models'
import * as API from '../../wailsjs/go/main/App'

// Create an Argo CD Application from YAML (prefilled template), validated by Argo CD.
export function CreateAppDialog({ statuses, defaultCtx, onClose, onCreated }: {
  statuses: store.ContextStatus[]
  defaultCtx?: string
  onClose: () => void
  onCreated: (key: string) => void
}) {
  const usable = statuses.filter((s) => s.state === 'ok')
  const [ctx, setCtx] = useState(defaultCtx && usable.some((s) => s.id === defaultCtx) ? defaultCtx : usable[0]?.id ?? '')
  const [yaml, setYaml] = useState('')
  const [upsert, setUpsert] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    if (ctx) API.NewAppTemplate(ctx).then(setYaml)
  }, [ctx])
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [onClose])

  const create = async () => {
    setBusy(true)
    setErr('')
    try {
      onCreated(await API.CreateApp(ctx, yaml, upsert))
    } catch (e) {
      setErr(String(e))
    }
    setBusy(false)
  }

  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal" style={{ width: 'min(820px, 94vw)' }}>
        <header>New application</header>
        <div className="content">
          {!usable.length && <div className="alert">Connect to an Argo CD instance first.</div>}
          <div className="row2col">
            <div className="field">
              <label>Argo CD instance</label>
              <select value={ctx} onChange={(e) => setCtx(e.target.value)}>
                {usable.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </div>
            <label className="check" style={{ marginTop: 22 }}>
              <input type="checkbox" checked={upsert} onChange={(e) => setUpsert(e.target.checked)} /> overwrite if it already exists (upsert)
            </label>
          </div>
          <div className="field">
            <label>Application manifest (YAML) — validated by Argo CD on create</label>
            <textarea className="code-block yaml-area" style={{ minHeight: '46vh' }} value={yaml} spellCheck={false} onChange={(e) => setYaml(e.target.value)} />
          </div>
          {err && <div className="alert">{err}</div>}
        </div>
        <footer>
          <span className="help" style={{ marginRight: 'auto' }}>Tip: for many similar apps, prefer an ApplicationSet in Git.</span>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy || !ctx} onClick={create}>{busy ? 'Creating…' : 'Create'}</button>
        </footer>
      </div>
    </div>
  )
}
