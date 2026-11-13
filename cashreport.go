package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const reportTitle = "LAPORAN KAS BANK - BANDUNG"

type ReportColumn struct {
	Key  AccountKey `json:"key"`
	Code string     `json:"code"`
	Name string     `json:"name"`
}
type ReportRow struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Number      string   `json:"number"`
	Check       string   `json:"check"`
	Amounts     []string `json:"amounts"`
}
type CashReport struct {
	Date     string         `json:"date"`
	Title    string         `json:"title"`
	Columns  []ReportColumn `json:"columns"`
	Rows     []ReportRow    `json:"rows"`
	Warnings []string       `json:"warnings"`
	Revision string         `json:"revision"`
}

func parseCents(value string) (*big.Int, error) {
	if !cashAmount.MatchString(value) {
		return nil, fmt.Errorf("nominal tidak valid: %q", value)
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.SplitN(value, ".", 2)
	fraction := "00"
	if len(parts) == 2 {
		fraction = (parts[1] + "00")[:2]
	}
	n, ok := new(big.Int).SetString(parts[0]+fraction, 10)
	if !ok {
		return nil, fmt.Errorf("nominal tidak valid")
	}
	if negative {
		n.Neg(n)
	}
	return n, nil
}
func centsString(n *big.Int) string {
	s := new(big.Int).Abs(n).String()
	for len(s) < 3 {
		s = "0" + s
	}
	s = s[:len(s)-2] + "." + s[len(s)-2:]
	if n.Sign() < 0 {
		s = "-" + s
	}
	return s
}
func centsArray(size int) []big.Int { return make([]big.Int, size) }
func reportSummary(kind, label string, values []big.Int) ReportRow {
	row := ReportRow{Kind: kind, Description: label, Amounts: make([]string, len(values))}
	for i := range values {
		row.Amounts[i] = centsString(&values[i])
	}
	return row
}

func (s *CashService) GetCashReport(date string) (CashReport, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02") != date {
		return CashReport{}, fmt.Errorf("Tanggal tidak valid. Pilih tanggal laporan yang benar.")
	}
	bank, err := s.ListAccounts()
	if err != nil {
		return CashReport{}, err
	}
	result := CashReport{Date: date, Title: reportTitle, Columns: []ReportColumn{}, Rows: []ReportRow{}, Warnings: []string{}}
	selected := map[string]int{}
	// ListAccounts returns newest first; the report uses physical insertion order.
	slices.Reverse(bank.Rows)
	for _, row := range bank.Rows {
		if row.Status != "LAPORAN" {
			continue
		}
		if _, exists := selected[row.Code1]; exists {
			return CashReport{}, fmt.Errorf("NOAC1 %s dipilih lebih dari sekali untuk laporan. Perbaiki centang di Daftar Akun.", row.Code1)
		}
		selected[row.Code1] = len(result.Columns)
		result.Columns = append(result.Columns, ReportColumn{Key: accountKey(row), Code: row.Code1, Name: row.Name})
	}
	if len(result.Columns) == 0 {
		return finishCashReport(result)
	}
	master, err := os.ReadFile(filepath.Join(s.dataDir, "DAFACC.DBF"))
	if err != nil {
		return CashReport{}, fmt.Errorf("DAFACC.DBF tidak dapat dibaca: %w", err)
	}
	opening, err := readOpeningBalances(master, result.Columns)
	if err != nil {
		return CashReport{}, fmt.Errorf("Saldo dasar DAFACC tidak dapat dibaca: %w", err)
	}
	dayIn, dayOut := centsArray(len(opening)), centsArray(len(opening))
	accIn, accOut := centsArray(len(opening)), centsArray(len(opening))
	var incoming, outgoing []ReportRow
	for _, source := range []struct {
		name, account string
		incoming      bool
	}{{"KAS_IN", "KPD1", true}, {"KAS_OUT", "KPK1", false}} {
		data, err := os.ReadFile(filepath.Join(s.dataDir, source.name+".DBF"))
		if err != nil {
			return CashReport{}, fmt.Errorf("%s.DBF tidak dapat dibaca: %w", source.name, err)
		}
		transactions, err := readReportTransactions(data, source.account, selected)
		if err != nil {
			return CashReport{}, fmt.Errorf("Gagal membaca %s.DBF: %w", source.name, err)
		}
		var memo []byte
		memoLoaded, memoMissing := false, false
		for _, transaction := range transactions {
			if transaction.Date > date {
				continue
			}
			index := selected[transaction.Account]
			amount, _ := parseCents(transaction.Amount) // validated by readReportTransactions
			acc, day := accOut, dayOut
			if source.incoming {
				acc, day = accIn, dayIn
			}
			acc[index].Add(&acc[index], amount)
			if transaction.Date < date {
				if source.incoming {
					opening[index].Add(&opening[index], amount)
				} else {
					opening[index].Sub(&opening[index], amount)
				}
				continue
			}
			day[index].Add(&day[index], amount)
			description := ""
			if strings.TrimLeft(transaction.memoPointer, "0") != "" {
				if !memoLoaded {
					memo, err = os.ReadFile(filepath.Join(s.dataDir, source.name+".DBT"))
					if os.IsNotExist(err) {
						memoMissing = true
						result.Warnings = append(result.Warnings, source.name+".DBT belum tersedia; keterangan sebagian transaksi belum dapat ditampilkan.")
					} else if err != nil {
						return CashReport{}, fmt.Errorf("%s.DBT tidak dapat dibaca: %w", source.name, err)
					}
					memoLoaded = true
				}
				if memoMissing {
					description = "Keterangan belum tersedia"
				} else {
					description, err = readCashMemo(memo, transaction.memoPointer)
					if err != nil {
						return CashReport{}, fmt.Errorf("Keterangan %s nomor %s tidak valid: %w", source.name, transaction.Number, err)
					}
				}
			}
			row := ReportRow{Kind: "transaction", Description: description, Number: transaction.Number, Amounts: make([]string, len(opening))}
			if !source.incoming && strings.HasPrefix(row.Description, ".") {
				row.Check = "."
				row.Description = strings.TrimPrefix(row.Description, ".")
			}
			row.Amounts[index] = centsString(amount)
			if source.incoming {
				incoming = append(incoming, row)
			} else {
				outgoing = append(outgoing, row)
			}
		}
	}
	order := func(a, b ReportRow) int { return strings.Compare(a.Number, b.Number) }
	slices.SortStableFunc(incoming, order)
	slices.SortStableFunc(outgoing, order)
	closing := centsArray(len(opening))
	for i := range closing {
		closing[i].Add(&opening[i], &dayIn[i])
		closing[i].Sub(&closing[i], &dayOut[i])
	}
	result.Rows = append(result.Rows, reportSummary("opening", "SALDO AWAL :", opening), ReportRow{Kind: "section", Description: "PENERIMAAN:"})
	result.Rows = append(result.Rows, incoming...)
	result.Rows = append(result.Rows, reportSummary("total", "TOTAL PENERIMAAN =", dayIn), reportSummary("accumulation", "AKUMULASI PENERIMAAN =", accIn), ReportRow{Kind: "spacer"}, ReportRow{Kind: "section", Description: "PENGELUARAN:"})
	result.Rows = append(result.Rows, outgoing...)
	result.Rows = append(result.Rows, reportSummary("total", "TOTAL PENGELUARAN =", dayOut), reportSummary("accumulation", "AKUMULASI PENGELUARAN =", accOut), ReportRow{Kind: "spacer"}, reportSummary("closing", "SALDO AKHIR =", closing))
	return finishCashReport(result)
}

func finishCashReport(report CashReport) (CashReport, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return CashReport{}, err
	}
	sum := sha256.Sum256(data)
	report.Revision = hex.EncodeToString(sum[:])
	return report, nil
}

func readOpeningBalances(data []byte, columns []ReportColumn) ([]big.Int, error) {
	master, err := readAccountRows(data)
	if err != nil {
		return nil, err
	}
	layout, err := readDBFLayout(data)
	if err != nil {
		return nil, err
	}
	f, ok := layout.fields["SAWD_00"]
	if !ok || f.kind != 'N' || f.decimals != 2 {
		return nil, fmt.Errorf("struktur SAWD_00 tidak didukung")
	}
	counts := map[AccountKey]int{}
	for _, row := range master.Rows {
		counts[accountKey(row)]++
	}
	wanted := map[AccountKey]int{}
	for i, column := range columns {
		if counts[column.Key] != 1 {
			return nil, fmt.Errorf("akun %s/%s/%s tidak ditemukan atau ambigu di master", column.Key.Code1, column.Key.Code2, column.Key.Code3)
		}
		wanted[column.Key] = i
	}
	result := centsArray(len(columns))
	for i := uint64(0); i < layout.count; i++ {
		row := data[layout.headerSize+int(i)*layout.recordSize : layout.headerSize+(int(i)+1)*layout.recordSize]
		if row[0] == '*' {
			continue
		}
		get := func(name string) string {
			field := layout.fields[name]
			return strings.TrimSpace(string(row[field.offset : field.offset+field.length]))
		}
		index, ok := wanted[AccountKey{get("NOAC1"), get("NOAC2"), get("NOAC3")}]
		if !ok {
			continue
		}
		value := strings.Trim(string(row[f.offset:f.offset+f.length]), " \x00")
		if value == "" {
			value = "0"
		}
		n, err := parseCents(value)
		if err != nil {
			return nil, fmt.Errorf("SAWD_00 akun %s tidak valid: %w", columns[index].Code, err)
		}
		result[index].Set(n)
	}
	return result, nil
}

func readReportTransactions(data []byte, accountField string, selected map[string]int) ([]CashRow, error) {
	layout, err := readDBFLayout(data)
	if err != nil {
		return nil, err
	}
	for _, expected := range []cashField{{name: "NO_BKAS", kind: 'C'}, {name: "TGL_BKAS", kind: 'D', length: 8}, {name: accountField, kind: 'C'}, {name: "JUMLAH", kind: 'N', decimals: 2}, {name: "KET", kind: 'M', length: 10}} {
		f, ok := layout.fields[expected.name]
		if !ok || f.kind != expected.kind || (expected.length != 0 && f.length != expected.length) || (f.kind == 'N' && f.decimals != 2) {
			return nil, fmt.Errorf("struktur field %s tidak didukung", expected.name)
		}
	}
	rows := []CashRow{}
	for i := uint64(0); i < layout.count; i++ {
		row := data[layout.headerSize+int(i)*layout.recordSize : layout.headerSize+(int(i)+1)*layout.recordSize]
		if row[0] == '*' {
			continue
		}
		if row[0] != ' ' {
			return nil, fmt.Errorf("penanda record %d tidak valid", i+1)
		}
		get := func(name string) string {
			f := layout.fields[name]
			return strings.TrimSpace(string(row[f.offset : f.offset+f.length]))
		}
		account := get(accountField)
		if _, ok := selected[account]; !ok {
			continue
		}
		date := get("TGL_BKAS")
		parsed, err := time.Parse("20060102", date)
		if err != nil || parsed.Year() < 1 || parsed.Format("20060102") != date {
			return nil, fmt.Errorf("tanggal record %d tidak valid", i+1)
		}
		if _, err := parseCents(get("JUMLAH")); err != nil {
			return nil, fmt.Errorf("JUMLAH record %d tidak valid: %w", i+1, err)
		}
		for _, c := range []byte(get("NO_BKAS")) {
			if c < 32 || c > 126 {
				return nil, fmt.Errorf("encoding NO_BKAS record %d tidak didukung", i+1)
			}
		}
		pointer := get("KET")
		if pointer != "" && !memoNumber.MatchString(pointer) {
			return nil, fmt.Errorf("referensi KET record %d tidak valid", i+1)
		}
		rows = append(rows, CashRow{Number: get("NO_BKAS"), Date: parsed.Format("2006-01-02"), Account: account, Amount: get("JUMLAH"), memoPointer: pointer})
	}
	return rows, nil
}
