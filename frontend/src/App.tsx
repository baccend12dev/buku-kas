import { useEffect, useRef, useState } from 'react'
import { CashService } from '../bindings/changeme'
import type { AccountResult, AccountRow, CashResult, CashReport } from '../bindings/changeme/models'
import AccountPicker from './AccountPicker'
import CashReportView from './CashReportView'

type Menu = 'masuk' | 'keluar' | 'akun' | 'laporan'

const menus: { id: Menu; label: string }[] = [
  { id: 'masuk', label: 'Kas Masuk' },
  { id: 'keluar', label: 'Kas Keluar' },
  { id: 'akun', label: 'Daftar Akun' },
  { id: 'laporan', label: 'Laporan' },
]

function MenuIcon({ menu }: { menu: Menu }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {menu === 'laporan' ? <><path d="M4 3v18h17M8 16v-4m5 4V8m5 8V5" /></> : menu === 'akun' ? <><rect x="4" y="3" width="16" height="18" rx="2" /><path d="M8 8h8M8 12h8M8 16h5" /></> : <><rect x="3" y="5" width="18" height="15" rx="2" /><path d="M3 10h5m8 0h5M12 3v11" /><path d={menu === 'masuk' ? 'm8 10 4 4 4-4' : 'm8 7 4-4 4 4'} /></>}
    </svg>
  )
}

function localDate() {
  const date = new Date()
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function displayDate(value: string) {
  return value ? value.split('-').reverse().join('/') : '—'
}

function displayAmount(value: string) {
  const [whole, fraction = ''] = value.split('.')
  return `${whole.replace(/\B(?=(\d{3})+(?!\d))/g, '.')},${fraction.padEnd(2, '0')}`
}

function App() {
  const [dateValue, setDateValue] = useState(localDate)
  const [activeMenu, setActiveMenu] = useState<Menu>('masuk')
  const [transactions, setTransactions] = useState<{ menu: Menu; date: string; data: CashResult } | null>(null)
  const [error, setError] = useState<{ menu: Menu; date: string; message: string } | null>(null)
  const [reload, setReload] = useState(0)
  const [accounts, setAccounts] = useState<AccountResult | null>(null)
  const [accountError, setAccountError] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [accountNotice, setAccountNotice] = useState('')
  const [statusBusy, setStatusBusy] = useState(false)
  const statusBusyRef = useRef(false)
  const [statusError, setStatusError] = useState('')
  const [report, setReport] = useState<CashReport | null>(null)
  const [reportError, setReportError] = useState<{ date: string; message: string } | null>(null)
  const [exporting, setExporting] = useState(false)
  const exportingRef = useRef(false)
  const [exportMessage, setExportMessage] = useState('')
  const [exportError, setExportError] = useState('')
  const selectedMenu = menus.find(menu => menu.id === activeMenu)!

  useEffect(() => {
    if ((activeMenu !== 'masuk' && activeMenu !== 'keluar') || !dateValue) return
    let cancelled = false
    setTransactions(null)
    setError(null)
    const load = activeMenu === 'masuk' ? CashService.ListCashIn : CashService.ListCashOut
    load(dateValue).then(data => {
      if (!cancelled) setTransactions({ menu: activeMenu, date: dateValue, data })
    }).catch((reason: unknown) => {
      if (!cancelled) setError({ menu: activeMenu, date: dateValue, message: reason instanceof Error ? reason.message : String(reason) })
    })
    return () => {
      cancelled = true
    }
  }, [activeMenu, dateValue, reload])

  useEffect(() => {
    if (activeMenu !== 'akun') return
    let cancelled = false
    setAccounts(null)
    setAccountError(null)
    CashService.ListAccounts().then(data => {
      if (!cancelled) setAccounts(data)
    }).catch((reason: unknown) => {
      if (!cancelled) setAccountError(reason instanceof Error ? reason.message : String(reason))
    })
    return () => { cancelled = true }
  }, [activeMenu, reload])

  useEffect(() => {
    if (activeMenu !== 'laporan' || !dateValue) return
    let cancelled = false
    setReport(null)
    setReportError(null)
    setExportMessage('')
    setExportError('')
    CashService.GetCashReport(dateValue).then(data => { if (!cancelled) setReport(data) }).catch((reason: unknown) => {
      if (!cancelled) setReportError({ date: dateValue, message: reason instanceof Error ? reason.message : String(reason) })
    })
    return () => { cancelled = true }
  }, [activeMenu, dateValue, reload])

  async function toggleReportAccount(row: AccountRow, enabled: boolean) {
    if (statusBusyRef.current) return
    statusBusyRef.current = true
    setStatusBusy(true)
    setStatusError('')
    const oldStatus = row.status
    const same = (candidate: AccountRow) => candidate.code1 === row.code1 && candidate.code2 === row.code2 && candidate.code3 === row.code3
    setAccounts(previous => previous && ({ ...previous, rows: previous.rows?.map(candidate => same(candidate) ? { ...candidate, status: enabled ? 'LAPORAN' : '' } : candidate) ?? [] }))
    try {
      await CashService.SetAccountReportEnabled({ code1: row.code1, code2: row.code2, code3: row.code3 }, enabled)
      setAccountNotice(`Pilihan laporan untuk ${row.code1} berhasil disimpan.`)
      setReport(null)
    } catch (reason) {
      setAccounts(previous => previous && ({ ...previous, rows: previous.rows?.map(candidate => same(candidate) ? { ...candidate, status: oldStatus } : candidate) ?? [] }))
      setStatusError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      statusBusyRef.current = false
      setStatusBusy(false)
    }
  }

  const currentReport = report?.date === dateValue ? report : null
  async function exportReport() {
    if (!currentReport || exportingRef.current) return
    exportingRef.current = true
    setExporting(true)
    setExportMessage('')
    setExportError('')
    try {
      const result = await CashService.ExportCashReport(currentReport.date, currentReport.revision)
      if (!result.cancelled) setExportMessage(`Laporan berhasil disimpan: ${result.path}`)
    } catch (reason) {
      setExportError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      exportingRef.current = false
      setExporting(false)
    }
  }

  const currentData = transactions?.date === dateValue && transactions.menu === activeMenu ? transactions.data : null
  const currentError = error?.date === dateValue && error.menu === activeMenu ? error.message : null
  const rows = currentData?.rows ?? []
  const accountRows = accounts?.rows ?? []
  const cashLabel = selectedMenu.label.toLocaleLowerCase('id-ID')

  return (
    <main className="app-shell">
      <header className="page-header">
        <div className="heading-group">
          <span className="app-mark" aria-hidden="true"><MenuIcon menu="akun" /></span>
          <div><p className="eyebrow">BUKU KAS</p><h1>Program Kas</h1></div>
        </div>
        <div className="today">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M16 3v4M8 3v4M3 11h18" /></svg>
          <div><label className="date-label" htmlFor="transaction-date">Tanggal transaksi</label><input id="transaction-date" type="date" value={dateValue} disabled={exporting} min="0001-01-01" max="9999-12-31" onChange={event => setDateValue(event.target.value)} /></div>
          <button className="today-button" type="button" disabled={exporting} onClick={() => setDateValue(localDate())}>Hari ini</button>
        </div>
      </header>

      <nav className="menu-grid" aria-label="Menu program kas">
        {menus.map(menu => (
          <button key={menu.id} type="button" disabled={statusBusy || exporting} className={`menu-card menu-${menu.id}${activeMenu === menu.id ? ' is-active' : ''}`} aria-pressed={activeMenu === menu.id} onClick={() => setActiveMenu(menu.id)} aria-controls="menu-content">
            <span className="menu-icon"><MenuIcon menu={menu.id} /></span>
            <span className="menu-title">{menu.label}</span>
          </button>
        ))}
      </nav>

      <section className="content-panel" id="menu-content" aria-labelledby="section-title">
        <div className="panel-toolbar">
          <h2 id="section-title">{selectedMenu.label}</h2>
          <div className="toolbar-actions">
          {activeMenu === 'akun' && <button className="primary-button" type="button" disabled={!accounts || statusBusy} onClick={() => setPickerOpen(true)}>+ Tambah Akun</button>}
          {activeMenu === 'laporan' && <button className="secondary-button" type="button" disabled={exporting} onClick={() => setReload(value => value + 1)}>Muat ulang</button>}
          <button className="export-button" type="button" disabled={activeMenu !== 'laporan' || !currentReport?.columns?.length || exporting} onClick={exportReport} aria-describedby="export-note">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 3v12m-4-4 4 4 4-4M4 16v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4" /></svg>
            {exporting ? 'Mengekspor…' : 'Export Excel'}
          </button>
          </div>
        </div>
        {activeMenu === 'akun' && accountNotice && <p className="account-notice" role="status">{accountNotice}</p>}
        {activeMenu === 'akun' && statusError && <p className="picker-error" role="alert">{statusError}</p>}
        {activeMenu === 'laporan' && exportError && <p className="picker-error" role="alert">{exportError}</p>}
        {activeMenu === 'laporan' && exportMessage && <p className="account-notice" role="status">{exportMessage}</p>}
        {activeMenu === 'laporan' ? !dateValue ? <div className="empty-state" role="status"><h3>Pilih tanggal laporan</h3></div>
          : reportError?.date === dateValue ? <div className="empty-state" role="alert"><h3>Laporan gagal dimuat</h3><p>{reportError.message}</p><button className="retry-button" type="button" onClick={() => setReload(value => value + 1)}>Coba lagi</button></div>
          : !currentReport ? <div className="empty-state" role="status"><h3>Memuat laporan…</h3></div>
          : !currentReport.columns?.length ? <div className="empty-state" role="status"><h3>Belum ada akun laporan</h3><p>Centang Laporan pada akun yang ingin ditampilkan di Daftar Akun.</p><button className="retry-button" type="button" onClick={() => setActiveMenu('akun')}>Buka Daftar Akun</button></div>
          : <CashReportView report={currentReport} /> : activeMenu !== 'akun' ? (
          !dateValue ? <div className="empty-state" role="status"><h3>Pilih tanggal transaksi</h3><p>Isi tanggal untuk menampilkan {cashLabel}.</p></div>
          : currentError ? <div className="empty-state" role="alert"><h3>Data {cashLabel} gagal dimuat</h3><p>{currentError}</p><button className="retry-button" type="button" onClick={() => setReload(value => value + 1)}>Coba lagi</button></div>
          : !currentData ? <div className="empty-state" role="status"><h3>Memuat {cashLabel}…</h3><p>Tanggal {displayDate(dateValue)}</p></div>
          : rows.length === 0 ? <div className="empty-state" role="status"><span className={`empty-icon menu-${activeMenu}`}><MenuIcon menu={activeMenu} /></span><h3>Tidak ada transaksi {cashLabel}</h3><p>Tanggal {displayDate(dateValue)}. Pilih tanggal lain untuk melihat transaksi.</p></div>
          : <>
            <div className="result-summary" role="status">{rows.length} transaksi · {displayDate(dateValue)} · Data terakhir di atas</div>
            <div key={`${activeMenu}-${dateValue}-${reload}`} className="table-scroll" tabIndex={0} role="region" aria-label={`Daftar transaksi ${cashLabel}`}>
              <table><caption className="sr-only">{selectedMenu.label} tanggal {displayDate(dateValue)}</caption><thead><tr><th scope="col">No. Bukti</th><th scope="col">Tanggal</th><th scope="col">{activeMenu === 'masuk' ? 'KPD1' : 'KPK1'}</th><th scope="col" className="amount">Jumlah</th><th scope="col">Keterangan</th></tr></thead>
                <tbody>{rows.map((row, index) => <tr key={`${activeMenu}-${row.number}-${index}`}><td>{row.number || '—'}</td><td>{displayDate(row.date)}</td><td>{row.account || '—'}</td><td className="amount">{displayAmount(row.amount)}</td><td className="description">{currentData.memoUnavailable ? 'Keterangan belum tersedia' : row.description || '—'}</td></tr>)}</tbody>
              </table>
            </div>
            {currentData.memoUnavailable && <p className="data-note">Keterangan belum dapat ditampilkan karena file memo {activeMenu === 'masuk' ? 'KAS_IN.DBT' : 'KAS_OUT.DBT'} belum tersedia.</p>}
          </>
        ) : accountError ? <div className="empty-state" role="alert"><h3>Daftar akun gagal dimuat</h3><p>{accountError}</p><button className="retry-button" type="button" onClick={() => setReload(value => value + 1)}>Coba lagi</button></div>
          : !accounts ? <div className="empty-state" role="status"><h3>Memuat daftar akun…</h3></div>
          : accountRows.length === 0 ? <div className="empty-state" role="status"><span className="empty-icon menu-akun"><MenuIcon menu="akun" /></span><h3>Belum ada akun aktif</h3><p>Gunakan Tambah Akun untuk memilih akun dari DAFACC.</p></div>
          : <>
            <div className="result-summary" role="status">{accountRows.length} akun · BANKLIST · Data terakhir di atas</div>
            <div key={`akun-${reload}`} className="table-scroll" tabIndex={0} role="region" aria-label="Daftar akun">
              <table><caption className="sr-only">Daftar akun, data terakhir ditampilkan lebih dahulu</caption><thead><tr><th scope="col">NOAC1</th><th scope="col">NOAC2</th><th scope="col">NOAC3</th><th scope="col">Nama Akun</th><th scope="col">Laporan</th></tr></thead>
                <tbody>{accountRows.map((row, index) => <tr key={`${row.code1}-${row.code2}-${row.code3}-${index}`}><td>{row.code1 || '—'}</td><td>{row.code2 || '—'}</td><td>{row.code3 || '—'}</td><td className="description">{row.name || '—'}</td><td><input type="checkbox" className="report-checkbox" aria-label={`Laporan ${row.code1}/${row.code2}/${row.code3} ${row.name}`} checked={row.status === 'LAPORAN'} disabled={statusBusy} onChange={event => toggleReportAccount(row, event.target.checked)} /></td></tr>)}</tbody>
              </table>
            </div>
          </>}
        <p className="panel-note" id="export-note">{activeMenu === 'laporan' ? 'Akun laporan mengikuti centang Laporan di Daftar Akun. Ekspor menggunakan format .xlsx.' : activeMenu === 'akun' ? 'Centang Laporan untuk menyertakan akun pada laporan harian. Export Excel tersedia di menu Laporan.' : 'Export Excel tersedia di menu Laporan.'}</p>
      </section>
      {pickerOpen && <AccountPicker onClose={() => setPickerOpen(false)} onAdded={result => {
        setPickerOpen(false)
        setAccountNotice(`${result.added} akun berhasil ditambahkan.${result.skipped ? ` ${result.skipped} pilihan sudah ada dan dilewati.` : ''}`)
        setAccounts(null)
        setReload(value => value + 1)
      }} />}
    </main>
  )
}

export default App
