import { useEffect, useRef, useState } from 'react'
import { CashService } from '../bindings/changeme'
import type { AccountRow, AddAccountsResult } from '../bindings/changeme/models'

const identity = (row: AccountRow) => JSON.stringify([row.code1, row.code2, row.code3])
const message = (reason: unknown) => reason instanceof Error ? reason.message : String(reason)

export default function AccountPicker({ onClose, onAdded }: { onClose: () => void; onAdded: (result: AddAccountsResult) => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const savingRef = useRef(false)
  const [data, setData] = useState<{ rows: AccountRow[]; existing: Set<string> } | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [reload, setReload] = useState(0)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Map<string, AccountRow>>(new Map())
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const element = dialog.current!
    element.showModal()
    return () => { element.close(); previous?.focus() }
  }, [])

  useEffect(() => {
    let cancelled = false
    setData(null)
    setLoadError(null)
    Promise.all([CashService.ListMasterAccounts(), CashService.ListAccounts()]).then(([master, bank]) => {
      if (cancelled) return
      const existing = new Set((bank.rows ?? []).map(identity))
      setData({ rows: master.rows ?? [], existing })
      setSelected(previous => new Map([...previous].filter(([key]) => !existing.has(key))))
    }).catch(reason => { if (!cancelled) setLoadError(message(reason)) })
    return () => { cancelled = true }
  }, [reload])

  const search = query.trim().toLocaleLowerCase('id-ID')
  const rows = data?.rows.filter(row => `${row.code1} ${row.code2} ${row.code3} ${row.code1}${row.code2}${row.code3} ${row.name}`.toLocaleLowerCase('id-ID').includes(search)) ?? []

  function toggle(row: AccountRow) {
    if (savingRef.current) return
    const key = identity(row)
    setSelected(previous => {
      const next = new Map(previous)
      if (next.has(key)) next.delete(key)
      else next.set(key, row)
      return next
    })
  }

  async function save() {
    if (savingRef.current || !data || selected.size === 0) return
    savingRef.current = true
    setSaving(true)
    setSaveError(null)
    try {
      const result = await CashService.AddBankAccounts([...selected.values()].map(row => ({ code1: row.code1, code2: row.code2, code3: row.code3 })))
      onAdded(result)
    } catch (reason) {
      setSaveError(message(reason))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  return <dialog ref={dialog} className="account-dialog" aria-labelledby="picker-title" aria-describedby="picker-description" onCancel={event => { event.preventDefault(); if (!savingRef.current) onClose() }}>
    <div className="picker-heading"><h2 id="picker-title">Tambah Akun</h2><p id="picker-description">Pilih akun dari DAFACC untuk ditambahkan ke daftar akun aktif.</p></div>
    <label className="picker-search">Cari kode atau nama akun<input autoFocus type="search" value={query} disabled={saving} onChange={event => setQuery(event.target.value)} placeholder="Ketik kode atau nama akun" /></label>
    {loadError ? <div className="empty-state" role="alert"><h3>Pilihan akun gagal dimuat</h3><p>{loadError}</p><button type="button" className="retry-button" onClick={() => setReload(value => value + 1)}>Coba lagi</button></div>
      : !data ? <div className="empty-state" role="status"><p>Memuat pilihan akun…</p></div>
      : rows.length === 0 ? <div className="empty-state" role="status"><p>{data.rows.length === 0 ? 'Tidak ada akun di DAFACC.' : 'Tidak ada akun yang cocok dengan pencarian.'}</p></div>
      : <div className="table-scroll picker-table" tabIndex={0} role="region" aria-label="Pilihan akun dari DAFACC" aria-busy={saving}>
        <table><thead><tr><th scope="col">Pilih</th><th scope="col">NOAC1</th><th scope="col">NOAC2</th><th scope="col">NOAC3</th><th scope="col">Nama Akun</th></tr></thead><tbody>{rows.map((row, index) => {
          const key = identity(row)
          const exists = data.existing.has(key)
          return <tr key={`${key}-${index}`} className={exists ? 'account-existing' : undefined}><td><input type="checkbox" aria-label={`Pilih ${row.code1}/${row.code2}/${row.code3} ${row.name}`} checked={exists || selected.has(key)} disabled={exists || saving} onChange={() => toggle(row)} /></td><td>{row.code1}</td><td>{row.code2}</td><td>{row.code3}</td><td className="description">{row.name}{exists && <span className="existing-note">Sudah ditambahkan</span>}</td></tr>
        })}</tbody></table>
      </div>}
    {saveError && <p className="picker-error" role="alert">{saveError}</p>}
    <div className="picker-footer"><span role="status">{selected.size} akun dipilih</span><div className="toolbar-actions"><button className="secondary-button" type="button" disabled={saving} onClick={() => { if (!savingRef.current) onClose() }}>Batal</button><button className="primary-button" type="button" disabled={saving || !data || selected.size === 0} onClick={save}>{saving ? 'Menyimpan…' : 'Tambahkan'}</button></div></div>
  </dialog>
}
