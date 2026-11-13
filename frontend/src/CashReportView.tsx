import type { CashReport } from '../bindings/changeme/models'

function amount(value: string) {
  const [whole, fraction = ''] = value.split('.')
  return `${whole.replace(/\B(?=(\d{3})+(?!\d))/g, '.')},${fraction.padEnd(2, '0')}`
}

export default function CashReportView({ report }: { report: CashReport }) {
  const columns = report.columns ?? []
  return <>
    <div className="report-heading"><h3>{report.title}</h3><p>Tanggal: {report.date.split('-').reverse().join('/')}</p></div>
    {(report.warnings ?? []).map(warning => <p key={warning} className="data-note" role="status">{warning}</p>)}
    <div className="table-scroll report-scroll" tabIndex={0} role="region" aria-label="Laporan kas harian" key={report.revision}>
      <table className="report-table"><caption className="sr-only">Laporan kas tanggal {report.date}</caption>
        <thead><tr><th scope="col" className="report-description">URAIAN</th><th scope="col">NO BKAS</th>{columns.map(column => <th scope="col" key={column.code} className="report-account">{column.name}<span>{column.code}</span></th>)}</tr></thead>
        <tbody>{(report.rows ?? []).map((row, index) => <tr key={index} className={`report-${row.kind}`}>
          {row.kind === 'section' || row.kind === 'spacer' ? <td colSpan={columns.length + 2}>{row.description || '\u00a0'}</td> : <>
            <td className="report-description">{row.description}</td><td>{row.number}</td>{columns.map((column, columnIndex) => <td className="amount" key={column.code}>{row.amounts?.[columnIndex] ? amount(row.amounts[columnIndex]) : ''}</td>)}
          </>}
        </tr>)}</tbody>
      </table>
    </div>
  </>
}
