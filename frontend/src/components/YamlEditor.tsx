import { useEffect, useState } from 'react'

// Minimal YAML editor: monospace textarea with tab-to-spaces, dirty tracking,
// save / revert, and an optional warning banner.
export function YamlEditor({
  load, save, warning, readOnlyNote,
}: {
  load: () => Promise<string>
  save?: (yaml: string) => Promise<void>
  warning?: React.ReactNode
  readOnlyNote?: string
}) {
  const [orig, setOrig] = useState<string | null>(null)
  const [text, setText] = useState('')
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const [editing, setEditing] = useState(false)

  const reload = () => {
    setErr('')
    load().then((y) => { setOrig(y); setText(y) }).catch((e) => setErr(String(e)))
  }
  useEffect(reload, [load]) // eslint-disable-line react-hooks/exhaustive-deps

  const dirty = orig !== null && text !== orig

  const doSave = async () => {
    if (!save) return
    setBusy(true)
    setErr('')
    setMsg('')
    try {
      await save(text)
      setMsg('Saved.')
      setEditing(false)
      reload()
    } catch (e) {
      setErr(String(e))
    }
    setBusy(false)
  }

  if (orig === null && !err) return <div className="help">Loading…</div>
  return (
    <div className="yaml-editor">
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 8 }}>
        {save && !editing && <button className="btn sm" onClick={() => setEditing(true)}>✎ Edit</button>}
        {editing && (
          <>
            <button className="btn sm primary" disabled={!dirty || busy} onClick={doSave}>{busy ? 'Saving…' : 'Save'}</button>
            <button className="btn sm" onClick={() => { setText(orig ?? ''); setEditing(false); setErr('') }}>Cancel</button>
            {dirty && <span className="muted-sm">unsaved changes</span>}
          </>
        )}
        {!save && readOnlyNote && <span className="muted-sm">{readOnlyNote}</span>}
        <span className="spacer" />
        <button className="btn sm" onClick={() => navigator.clipboard?.writeText(text)}>Copy</button>
        {!editing && <button className="btn sm" onClick={reload}>Reload</button>}
      </div>
      {editing && warning}
      {err && <div className="alert" style={{ marginBottom: 8 }}>{err}</div>}
      {msg && <div className="alert ok" style={{ marginBottom: 8 }}>{msg}</div>}
      <textarea
        className="code-block yaml-area"
        value={text}
        readOnly={!editing}
        spellCheck={false}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Tab') {
            e.preventDefault()
            const el = e.currentTarget
            const s = el.selectionStart
            const v = text.slice(0, s) + '  ' + text.slice(el.selectionEnd)
            setText(v)
            requestAnimationFrame(() => { el.selectionStart = el.selectionEnd = s + 2 })
          }
          if ((e.metaKey || e.ctrlKey) && e.key === 's') {
            e.preventDefault()
            if (editing && dirty) doSave()
          }
        }}
      />
    </div>
  )
}
