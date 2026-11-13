package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func reportMasterFixture(rows []AccountRow, balances []string) []byte {
	data := bankFixture(rows...)
	// Same four identity fields, followed by SAWD_00 N(18,2).
	data = append([]byte(nil), data[:193]...)
	copy(data[160:171], make([]byte, 11))
	copy(data[160:171], "SAWD_00")
	data[171], data[176], data[177] = 'N', 18, 2
	binary.LittleEndian.PutUint16(data[10:12], 81)
	for i, row := range rows {
		record := bytes.Repeat([]byte{' '}, 81)
		copy(record[1:6], row.Code1)
		copy(record[6:9], row.Code2)
		copy(record[9:11], row.Code3)
		copy(record[11:63], row.Name)
		copy(record[63:], balances[i])
		data = append(data, record...)
	}
	return append(data, 0x1a)
}

func reportServiceFixture(t *testing.T, count int) *CashService {
	t.Helper()
	dir := t.TempDir()
	rows := []AccountRow{}
	balances := []string{}
	for i := 0; i < count; i++ {
		rows = append(rows, AccountRow{Code1: fmt.Sprintf("%05d", i+1), Code2: "000", Code3: "01", Name: fmt.Sprintf("Bank %d", i+1), Status: "LAPORAN"})
		balances = append(balances, "0.00")
	}
	writeTestFile(t, filepath.Join(dir, "BANKLIST.DBF"), bankFixture(rows...))
	writeTestFile(t, filepath.Join(dir, "DAFACC.DBF"), reportMasterFixture(rows, balances))
	writeTestFile(t, filepath.Join(dir, "KAS_IN.DBF"), cashFixture())
	writeTestFile(t, filepath.Join(dir, "KAS_OUT.DBF"), cashFixture())
	return &CashService{dataDir: dir}
}

func summaryByLabel(t *testing.T, report CashReport, label string) ReportRow {
	t.Helper()
	for _, row := range report.Rows {
		if row.Description == label {
			return row
		}
	}
	t.Fatalf("summary not found: %s", label)
	return ReportRow{}
}

func TestReportStatusPersistenceBackupAndDuplicateCode(t *testing.T) {
	s := reportServiceFixture(t, 2)
	path := filepath.Join(s.dataDir, "BANKLIST.DBF")
	rows := []AccountRow{{Code1: "00001", Code2: "000", Code3: "01", Name: "One"}, {Code1: "00001", Code2: "000", Code3: "02", Name: "Other"}}
	before := bankFixture(rows...)
	writeTestFile(t, path, before)
	key := accountKey(rows[0])
	if err := s.SetAccountReportEnabled(key, true); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, readTestFile(t, path+".bak")) {
		t.Fatal("status backup is not original")
	}
	after := readTestFile(t, path)
	if string(after[193+63:193+73]) != "LAPORAN   " {
		t.Fatal("status not persisted")
	}
	if !bytes.Equal(before[193:193+63], after[193:193+63]) || !bytes.Equal(before[193+73:], after[193+73:]) {
		t.Fatal("unrelated record data changed")
	}
	listed, err := (&CashService{dataDir: s.dataDir}).ListAccounts()
	if err != nil || listed.Rows[1].Status != "LAPORAN" {
		t.Fatal("persisted status missing", err)
	}
	if err := s.SetAccountReportEnabled(accountKey(rows[1]), true); err == nil {
		t.Fatal("duplicate NOAC1 accepted")
	}
	if !bytes.Equal(after, readTestFile(t, path)) {
		t.Fatal("failed status change changed file")
	}
	if err := s.SetAccountReportEnabled(key, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountReportEnabled(accountKey(rows[1]), true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountReportEnabled(AccountKey{Code1: "99999"}, true); err == nil {
		t.Fatal("missing identity accepted")
	}
	unlock, err := lockBankFile(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountReportEnabled(key, true); err == nil {
		unlock()
		t.Fatal("status update ignored lock")
	}
	unlock()
}

func TestReportStatusBackupFailure(t *testing.T) {
	s := reportServiceFixture(t, 1)
	path := filepath.Join(s.dataDir, "BANKLIST.DBF")
	before := readTestFile(t, path)
	if err := os.Mkdir(path+".bak", 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountReportEnabled(AccountKey{"00001", "000", "01"}, false); err == nil {
		t.Fatal("backup failure ignored")
	}
	if !bytes.Equal(before, readTestFile(t, path)) {
		t.Fatal("status failure changed original")
	}
}

func TestReportLegacyMathAndDateCutoffs(t *testing.T) {
	s := reportServiceFixture(t, 2)
	rows := []AccountRow{{Code1: "00001", Code2: "000", Code3: "01", Name: "One", Status: "LAPORAN"}, {Code1: "00002", Code2: "000", Code3: "01", Name: "Negative", Status: "LAPORAN"}}
	master := reportMasterFixture(rows, []string{"100.01\x00\x00", "-50.00"})
	writeTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF"), master)
	writeTestFile(t, filepath.Join(s.dataDir, "KAS_IN.DBF"), cashFixture(
		map[string]string{"NO_BKAS": "PRE", "TGL_BKAS": "20261109", "KPD1": "00001", "JUMLAH": "10.10"},
		map[string]string{"NO_BKAS": "BM002", "TGL_BKAS": "20261110", "KPD1": "00001", "JUMLAH": "0.20"},
		map[string]string{"NO_BKAS": "BM001", "TGL_BKAS": "20261110", "KPD1": "00001", "JUMLAH": "0.10"},
		map[string]string{"NO_BKAS": "FUTURE", "TGL_BKAS": "20261111", "KPD1": "00001", "JUMLAH": "999.00"},
		map[string]string{"NO_BKAS": "DELETED", "TGL_BKAS": "20261110", "KPD1": "00001", "JUMLAH": "999.00", "deleted": "yes"},
		map[string]string{"NO_BKAS": "OTHER", "TGL_BKAS": "20261110", "KPD1": "99999", "JUMLAH": "999.00"},
	))
	writeTestFile(t, filepath.Join(s.dataDir, "KAS_OUT.DBF"), cashFixture(
		map[string]string{"NO_BKAS": "PRE", "TGL_BKAS": "20261109", "KPK1": "00001", "JUMLAH": "5.05"},
		map[string]string{"NO_BKAS": "BK001", "TGL_BKAS": "20261110", "KPK1": "00001", "JUMLAH": "2.02", "KET": "1"},
	))
	memo := make([]byte, 512)
	memo = append(memo, []byte(".Cek bank\x1a")...)
	writeTestFile(t, filepath.Join(s.dataDir, "KAS_OUT.DBT"), memo)
	report, err := s.GetCashReport("2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	if report.Columns[0].Code != "00001" || report.Columns[1].Code != "00002" {
		t.Fatal("wrong physical column order")
	}
	for label, want := range map[string]string{"SALDO AWAL :": "105.06", "TOTAL PENERIMAAN =": "0.30", "TOTAL PENGELUARAN =": "2.02", "AKUMULASI PENERIMAAN =": "10.40", "AKUMULASI PENGELUARAN =": "7.07", "SALDO AKHIR =": "103.34"} {
		if got := summaryByLabel(t, report, label).Amounts[0]; got != want {
			t.Errorf("%s: %s != %s", label, got, want)
		}
	}
	if summaryByLabel(t, report, "SALDO AKHIR =").Amounts[1] != "-50.00" {
		t.Fatal("negative balance lost")
	}
	var numbers []string
	for _, row := range report.Rows {
		if row.Kind == "transaction" {
			numbers = append(numbers, row.Number)
			if row.Amounts[1] != "" {
				t.Fatal("unrelated amount cell must be blank")
			}
			if row.Number == "BK001" && (row.Check != "." || row.Description != "Cek bank") {
				t.Fatal("check marker rule lost")
			}
		}
	}
	if strings.Join(numbers, ",") != "BM001,BM002,BK001" {
		t.Fatal("wrong transaction sort", numbers)
	}
	if len(report.Warnings) != 0 {
		t.Fatal(report.Warnings)
	}
	if !bytes.Equal(master, readTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF"))) {
		t.Fatal("master modified")
	}
	quiet, err := s.GetCashReport("2026-11-08")
	if err != nil || summaryByLabel(t, quiet, "SALDO AWAL :").Amounts[0] != "100.01" || summaryByLabel(t, quiet, "TOTAL PENERIMAAN =").Amounts[0] != "0.00" {
		t.Fatal("empty date math", err)
	}
}

func TestReportColumnCountsAndMissingMemo(t *testing.T) {
	for _, count := range []int{0, 1, 6, 8} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s := reportServiceFixture(t, count)
			report, err := s.GetCashReport("2026-11-10")
			if err != nil || len(report.Columns) != count {
				t.Fatal("dynamic columns", err)
			}
			if count == 0 && len(report.Rows) != 0 {
				t.Fatal("empty selection should have no rows")
			}
			if count > 0 && len(report.Rows) != 10 {
				t.Fatal("empty date should retain all summaries", len(report.Rows))
			}
		})
	}
	s := reportServiceFixture(t, 1)
	writeTestFile(t, filepath.Join(s.dataDir, "KAS_IN.DBF"), cashFixture(map[string]string{"TGL_BKAS": "20261110", "KPD1": "00001", "JUMLAH": "1.00", "KET": "1"}))
	report, err := s.GetCashReport("2026-11-10")
	if err != nil || len(report.Warnings) != 1 || summaryByLabel(t, report, "TOTAL PENERIMAAN =").Amounts[0] != "1.00" {
		t.Fatal("missing memo must preserve totals", err)
	}
}

func TestReportRejectsInvalidSourcesAndAmbiguity(t *testing.T) {
	for _, row := range []map[string]string{{"TGL_BKAS": "20260230", "KPD1": "00001", "JUMLAH": "1.00"}, {"TGL_BKAS": "20261110", "KPD1": "00001", "JUMLAH": "not-money"}} {
		s := reportServiceFixture(t, 1)
		writeTestFile(t, filepath.Join(s.dataDir, "KAS_IN.DBF"), cashFixture(row))
		if _, err := s.GetCashReport("2026-11-10"); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	s := reportServiceFixture(t, 1)
	if _, err := s.GetCashReport("2026-02-30"); err == nil {
		t.Fatal("invalid request date accepted")
	}
	row := AccountRow{Code1: "00001", Code2: "000", Code3: "01", Status: "LAPORAN"}
	writeTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF"), reportMasterFixture([]AccountRow{row, row}, []string{"1.00", "2.00"}))
	if _, err := s.GetCashReport("2026-11-10"); err == nil {
		t.Fatal("ambiguous master accepted")
	}
	writeTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF"), reportMasterFixture([]AccountRow{{Code1: "00001", Code2: "000", Code3: "02"}}, []string{"1.00"}))
	if _, err := s.GetCashReport("2026-11-10"); err == nil {
		t.Fatal("wrong full master identity accepted")
	}
	writeTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF"), reportMasterFixture([]AccountRow{row}, []string{"garbage"}))
	if _, err := s.GetCashReport("2026-11-10"); err == nil {
		t.Fatal("invalid opening balance accepted")
	}
	writeTestFile(t, filepath.Join(s.dataDir, "BANKLIST.DBF"), bankFixture(row, row))
	if _, err := s.GetCashReport("2026-11-10"); err == nil {
		t.Fatal("duplicate selected NOAC1 accepted")
	}
}

func TestReportMoneyExactAndExcelPrecision(t *testing.T) {
	a, _ := parseCents("9999999999999999999999.99")
	b, _ := parseCents("0.01")
	if got := centsString(new(big.Int).Add(a, b)); got != "10000000000000000000000.00" {
		t.Fatal(got)
	}
	for _, value := range []string{"1234567890123.45", "-50000000000.00", "0.01"} {
		if err := validateReportExcelPrecision(CashReport{Rows: []ReportRow{{Amounts: []string{value}}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateReportExcelPrecision(CashReport{Rows: []ReportRow{{Amounts: []string{"12345678901234.56"}}}}); err == nil {
		t.Fatal("precision loss accepted")
	}
}

func TestReportExportWorkbookAndRevision(t *testing.T) {
	s := reportServiceFixture(t, 8)
	report, err := s.GetCashReport("2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := s.exportCashReport(report.Date, "old", func(string) (string, error) { called = true; return "", nil }); err == nil || called {
		t.Fatal("stale report opened dialog")
	}
	cancelled, err := s.exportCashReport(report.Date, report.Revision, func(name string) (string, error) {
		if name != "Laporan_Kas_2026-11-10.xlsx" {
			t.Fatal(name)
		}
		return "", nil
	})
	if err != nil || !cancelled.Cancelled {
		t.Fatal("cancel is not a failure", err)
	}
	path := filepath.Join(t.TempDir(), "report.xlsx")
	_, err = s.exportCashReport(report.Date, report.Revision, func(string) (string, error) {
		if err := s.SetAccountReportEnabled(AccountKey{"00008", "000", "01"}, false); err != nil {
			t.Fatal(err)
		}
		return path, nil
	})
	if err == nil {
		t.Fatal("change during dialog accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale export wrote a file")
	}
	if err := s.SetAccountReportEnabled(AccountKey{"00008", "000", "01"}, true); err != nil {
		t.Fatal(err)
	}
	report, err = s.GetCashReport(report.Date)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.exportCashReport(report.Date, report.Revision, func(string) (string, error) { return path, nil })
	if err != nil || result.Path != path {
		t.Fatal("export failed", err)
	}
	book, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	const sheet = "Laporan Kas"
	if got, err := book.GetCellValue(sheet, "K5"); err != nil || got != "00008" {
		t.Fatal("dynamic final column", got, err)
	}
	if visible, err := book.GetColVisible(sheet, "C"); err != nil || visible {
		t.Fatal("check column must be hidden")
	}
	for _, column := range []string{"D", "E", "F", "G", "H", "I", "J", "K"} {
		if visible, err := book.GetColVisible(sheet, column); err != nil || !visible {
			t.Fatal("selected account hidden", column, err)
		}
	}
	if kind, err := book.GetCellType(sheet, "D5"); err != nil || kind != excelize.CellTypeSharedString {
		t.Fatal("account code must be text", kind, err)
	}
	// OOXML defaults a missing t attribute to numeric; SetCellDefault keeps the exact decimal literal.
	if kind, err := book.GetCellType(sheet, "D6"); err != nil || (kind != excelize.CellTypeNumber && kind != excelize.CellTypeUnset) {
		t.Fatal("amount must be numeric", kind, err)
	}
	if value, err := book.GetCellValue(sheet, "D6"); err != nil || value != "0.00" {
		t.Fatal("two decimals", value, err)
	}
	names := book.GetDefinedName()
	if len(names) != 2 {
		t.Fatal("print area and repeat titles missing", names)
	}
	for cell, color := range map[string]string{"A4": "D9D9D9", "A6": "F2F2F2", "A8": "FFFFCC", "A9": "D9E2F3", "A15": "E2EFDA"} {
		id, err := book.GetCellStyle(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		style, err := book.GetStyle(id)
		if err != nil || len(style.Fill.Color) != 1 || style.Fill.Color[0] != color {
			t.Fatalf("wrong summary/header style %s: %+v %v", cell, style, err)
		}
	}
	merges, err := book.GetMergeCells(sheet)
	if err != nil || len(merges) != 2 || merges[0].GetEndAxis() != "K1" {
		t.Fatal("title merge must use dynamic final column", merges, err)
	}
}

func TestReportExportFailurePreservesDestination(t *testing.T) {
	s := reportServiceFixture(t, 1)
	report, err := s.GetCashReport("2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "target.xlsx")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "keep.txt")
	writeTestFile(t, marker, []byte("unchanged"))
	if _, err := s.exportCashReport(report.Date, report.Revision, func(string) (string, error) { return path, nil }); err == nil {
		t.Fatal("directory replacement accepted")
	}
	if string(readTestFile(t, marker)) != "unchanged" {
		t.Fatal("destination damaged")
	}
}

func TestReportExportKeepsExactFractionalAmounts(t *testing.T) {
	s := reportServiceFixture(t, 1)
	report, err := s.GetCashReport("2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	values := []string{"0.10", "-50000000000.01", "9999999999999.99"}
	for i, value := range values {
		report.Rows[i] = ReportRow{Kind: "transaction", Description: "Fraction", Number: "00001", Amounts: []string{value}}
	}
	book, err := buildReportWorkbook(report)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := excelize.OpenReader(buffer)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for i, wanted := range values {
		cell := fmt.Sprintf("D%d", i+6)
		actual, err := reopened.GetCellValue("Laporan Kas", cell, excelize.Options{RawCellValue: true})
		if err != nil || actual != wanted {
			t.Fatalf("fraction changed: %s %s %s %v", cell, actual, wanted, err)
		}
	}
}
