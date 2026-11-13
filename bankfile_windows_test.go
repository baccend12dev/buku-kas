package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBankWindowsReplacementFailureKeepsOriginal(t *testing.T) {
	s, path, before := bankServiceFixture(t)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// A reader without FILE_SHARE_DELETE prevents replacement but allows reads.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if _, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}}); err == nil {
		t.Fatal("replacement should fail while target is held")
	}
	if !bytes.Equal(before, readTestFile(t, path)) || !bytes.Equal(before, readTestFile(t, path+".bak")) {
		t.Fatal("replacement failure damaged original/backup")
	}
}

func TestReportWindowsLockedFilesPreservePreviousData(t *testing.T) {
	s := reportServiceFixture(t, 1)
	bankPath := filepath.Join(s.dataDir, "BANKLIST.DBF")
	before := readTestFile(t, bankPath)
	name, err := windows.UTF16PtrFromString(bankPath)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountReportEnabled(AccountKey{"00001", "000", "01"}, false); err == nil {
		windows.CloseHandle(handle)
		t.Fatal("locked STATUS replacement accepted")
	}
	windows.CloseHandle(handle)
	if !bytes.Equal(before, readTestFile(t, bankPath)) {
		t.Fatal("locked STATUS write changed bank")
	}
	report, err := s.GetCashReport("2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.xlsx")
	_, err = s.exportCashReport(report.Date, report.Revision, func(string) (string, error) { return path, nil })
	if err != nil {
		t.Fatal(err)
	}
	original := readTestFile(t, path)
	name, err = windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err = windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if _, err := s.exportCashReport(report.Date, report.Revision, func(string) (string, error) { return path, nil }); err == nil {
		t.Fatal("locked export replacement accepted")
	}
	if !bytes.Equal(original, readTestFile(t, path)) {
		t.Fatal("locked export damaged previous workbook")
	}
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".cash-report-*"))
	if err != nil || len(staged) != 0 {
		t.Fatal("staging file not cleaned", staged, err)
	}
}
