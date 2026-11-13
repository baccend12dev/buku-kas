package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type AccountRow struct {
	Code1  string `json:"code1"`
	Code2  string `json:"code2"`
	Code3  string `json:"code3"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type AccountResult struct {
	Rows []AccountRow `json:"rows"`
}

func (s *CashService) ListAccounts() (AccountResult, error) {
	return s.listAccountsFile("BANKLIST.DBF")
}

func (s *CashService) ListMasterAccounts() (AccountResult, error) {
	return s.listAccountsFile("DAFACC.DBF")
}

func (s *CashService) listAccountsFile(name string) (AccountResult, error) {
	if s.dataDir == "" {
		return AccountResult{}, fmt.Errorf("Lokasi aplikasi tidak dapat ditentukan. File DBF dan DBT harus berada di folder yang sama dengan executable.")
	}
	data, err := os.ReadFile(filepath.Join(s.dataDir, name))
	if err != nil {
		return AccountResult{}, fmt.Errorf("%s tidak dapat dibaca. Letakkan file langsung di folder yang sama dengan executable dan periksa izin akses. Lokasi: %s", name, s.dataDir)
	}
	if name == "BANKLIST.DBF" {
		if _, err := bankLayout(data); err != nil {
			return AccountResult{}, fmt.Errorf("Gagal membaca BANKLIST.DBF: %w", err)
		}
	}
	result, err := readAccountRows(data)
	if err != nil {
		return AccountResult{}, fmt.Errorf("Gagal membaca %s: %w", name, err)
	}
	return result, nil
}

func readAccountRows(data []byte) (AccountResult, error) {
	result := AccountResult{Rows: []AccountRow{}}
	layout, err := readDBFLayout(data)
	if err != nil {
		return AccountResult{}, err
	}
	for name, length := range map[string]int{"NOAC1": 5, "NOAC2": 3, "NOAC3": 2, "NAMA": 52} {
		f, ok := layout.fields[name]
		if !ok || f.kind != 'C' || f.length != length {
			return AccountResult{}, fmt.Errorf("struktur field %s tidak didukung", name)
		}
	}
	for i := uint64(0); i < layout.count; i++ {
		start := layout.headerSize + int(i)*layout.recordSize
		row := data[start : start+layout.recordSize]
		if row[0] == '*' {
			continue
		}
		if row[0] != ' ' {
			return AccountResult{}, fmt.Errorf("penanda record %d tidak valid", i+1)
		}
		values := map[string]string{}
		for _, name := range []string{"NOAC1", "NOAC2", "NOAC3", "NAMA"} {
			f := layout.fields[name]
			value := strings.TrimSpace(string(row[f.offset : f.offset+f.length]))
			for _, c := range []byte(value) {
				if c < 32 || c > 126 {
					return AccountResult{}, fmt.Errorf("encoding field %s pada record %d belum didukung", name, i+1)
				}
			}
			values[name] = value
		}
		status := ""
		if field, ok := layout.fields["STATUS"]; ok {
			status = strings.TrimSpace(string(row[field.offset : field.offset+field.length]))
			for _, c := range []byte(status) {
				if c < 32 || c > 126 {
					return AccountResult{}, fmt.Errorf("encoding STATUS pada record %d belum didukung", i+1)
				}
			}
		}
		result.Rows = append(result.Rows, AccountRow{Code1: values["NOAC1"], Code2: values["NOAC2"], Code3: values["NOAC3"], Name: values["NAMA"], Status: status})
	}
	// Last means the last active record in the original DBF, not a computed date.
	slices.Reverse(result.Rows)
	return result, nil
}
