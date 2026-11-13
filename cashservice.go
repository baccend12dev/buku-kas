package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CashService reads cash and master data and adds selected accounts to BANKLIST.
type CashService struct{ dataDir string }

type CashRow struct {
	Number      string `json:"number"`
	Date        string `json:"date"`
	Account     string `json:"account"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
	memoPointer string
}

type CashResult struct {
	Rows            []CashRow `json:"rows"`
	MemoUnavailable bool      `json:"memoUnavailable"`
}

func NewCashService() *CashService {
	// Data files live directly beside the executable, regardless of working directory.
	exe, err := os.Executable()
	if err != nil {
		return &CashService{}
	}
	return &CashService{dataDir: filepath.Dir(exe)}
}

func (s *CashService) ListCashIn(date string) (CashResult, error) {
	return s.listCash("KAS_IN", "KPD1", date)
}
func (s *CashService) ListCashOut(date string) (CashResult, error) {
	return s.listCash("KAS_OUT", "KPK1", date)
}

func (s *CashService) listCash(table, accountField, date string) (CashResult, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02") != date {
		return CashResult{}, fmt.Errorf("Tanggal tidak valid. Pilih tanggal transaksi yang benar.")
	}
	if s.dataDir == "" {
		return CashResult{}, fmt.Errorf("Lokasi aplikasi tidak dapat ditentukan. File DBF dan DBT harus berada di folder yang sama dengan executable.")
	}
	data, err := os.ReadFile(filepath.Join(s.dataDir, table+".DBF"))
	if err != nil {
		return CashResult{}, fmt.Errorf("%s.DBF tidak dapat dibaca. Letakkan file langsung di folder yang sama dengan executable dan periksa izin akses. Lokasi: %s", table, s.dataDir)
	}
	result, err := readCashRows(data, parsed.Format("20060102"), accountField)
	if err != nil {
		return CashResult{}, fmt.Errorf("Gagal membaca %s.DBF: %w", table, err)
	}
	needsMemo := false
	for _, row := range result.Rows {
		if row.memoPointer != "" && strings.TrimLeft(row.memoPointer, "0") != "" {
			needsMemo = true
			break
		}
	}
	if !needsMemo {
		return result, nil
	}
	memo, err := os.ReadFile(filepath.Join(s.dataDir, table+".DBT"))
	if os.IsNotExist(err) {
		result.MemoUnavailable = true
		return result, nil
	}
	if err != nil {
		return CashResult{}, fmt.Errorf("%s.DBT tidak dapat dibaca. Periksa izin akses.", table)
	}
	for i := range result.Rows {
		description, err := readCashMemo(memo, result.Rows[i].memoPointer)
		if err != nil {
			return CashResult{}, fmt.Errorf("Gagal membaca keterangan %s.DBT pada transaksi ke-%d: %w", table, i+1, err)
		}
		result.Rows[i].Description = description
	}
	return result, nil
}

type cashField struct {
	name                     string
	kind                     byte
	length, decimals, offset int
}

var cashAmount = regexp.MustCompile(`^-?\d+(\.\d{1,2})?$`)
var memoNumber = regexp.MustCompile(`^\d+$`)

func readCashMemo(memo []byte, pointer string) (string, error) {
	if pointer == "" {
		return "", nil
	}
	if !memoNumber.MatchString(pointer) {
		return "", fmt.Errorf("referensi memo tidak valid")
	}
	block, err := strconv.ParseUint(pointer, 10, 64)
	if err != nil {
		return "", fmt.Errorf("referensi memo tidak valid")
	}
	if block == 0 {
		return "", nil
	}
	// dBASE III memo blocks are 512 bytes. The local files terminate text with
	// Ctrl-Z (often doubled); stop at the first marker to exclude stale padding.
	if len(memo) < 512 || block > uint64((len(memo)-1)/512) {
		return "", fmt.Errorf("referensi memo di luar batas file")
	}
	text := memo[int(block)*512:]
	end := bytes.IndexByte(text, 0x1a)
	if end < 0 {
		return "", fmt.Errorf("teks memo tidak memiliki penanda akhir")
	}
	text = text[:end]
	for _, c := range text {
		if c > 126 || (c < 32 && c != '\r' && c != '\n' && c != '\t') {
			return "", fmt.Errorf("encoding teks memo belum didukung")
		}
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(text), "\r\n", "\n"), "\r", "\n")), nil
}

// Only the dBASE III layout present in the local cash tables is supported.
func readCashRows(data []byte, date, accountField string) (CashResult, error) {
	result := CashResult{Rows: []CashRow{}}
	invalid := func() (CashResult, error) {
		return CashResult{}, fmt.Errorf("struktur file tidak valid atau tidak didukung")
	}
	if accountField != "KPD1" && accountField != "KPK1" {
		return invalid()
	}
	layout, err := readDBFLayout(data)
	if err != nil {
		return CashResult{}, err
	}
	headerSize, recordSize, count, fields := layout.headerSize, layout.recordSize, layout.count, layout.fields
	for _, expected := range []cashField{
		{name: "NO_BKAS", kind: 'C'}, {name: "TGL_BKAS", kind: 'D', length: 8},
		{name: accountField, kind: 'C'}, {name: "JUMLAH", kind: 'N', decimals: 2}, {name: "KET", kind: 'M', length: 10},
	} {
		field, ok := fields[expected.name]
		if !ok || field.kind != expected.kind || (expected.length != 0 && field.length != expected.length) || (field.kind == 'N' && field.decimals != expected.decimals) {
			return invalid()
		}
	}
	for n := uint64(0); n < count; n++ {
		start := headerSize + int(n)*recordSize
		row := data[start : start+recordSize]
		if row[0] == '*' {
			continue
		}
		if row[0] != ' ' {
			return invalid()
		}
		get := func(name string) string {
			f := fields[name]
			return strings.TrimSpace(string(row[f.offset : f.offset+f.length]))
		}
		if get("TGL_BKAS") != date {
			continue
		}
		for _, name := range []string{"NO_BKAS", accountField} {
			for _, c := range []byte(get(name)) {
				if c < 32 || c > 126 {
					return CashResult{}, fmt.Errorf("encoding field %s belum didukung", name)
				}
			}
		}
		amount := get("JUMLAH")
		if !cashAmount.MatchString(amount) {
			return CashResult{}, fmt.Errorf("nilai JUMLAH pada record %d tidak valid", n+1)
		}
		pointer := get("KET")
		if pointer != "" && !memoNumber.MatchString(pointer) {
			return CashResult{}, fmt.Errorf("referensi KET pada record %d tidak valid", n+1)
		}
		result.Rows = append(result.Rows, CashRow{Number: get("NO_BKAS"), Date: date[:4] + "-" + date[4:6] + "-" + date[6:], Account: get(accountField), Amount: amount, memoPointer: pointer})
	}
	slices.Reverse(result.Rows)
	return result, nil
}
