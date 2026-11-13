package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *CashService) SetAccountReportEnabled(key AccountKey, enabled bool) error {
	if s.dataDir == "" {
		return fmt.Errorf("Lokasi aplikasi tidak dapat ditentukan. File DBF dan DBT harus berada di folder yang sama dengan executable.")
	}
	path := filepath.Join(s.dataDir, "BANKLIST.DBF")
	unlock, err := lockBankFile(path + ".lock")
	if err != nil {
		return fmt.Errorf("BANKLIST sedang digunakan atau tidak dapat dikunci. Coba lagi: %w", err)
	}
	defer unlock()
	before, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("BANKLIST.DBF tidak dapat dibaca: %w", err)
	}
	layout, err := bankLayout(before)
	if err != nil {
		return err
	}
	bank, err := readAccountRows(before)
	if err != nil {
		return err
	}
	matches := 0
	for _, row := range bank.Rows {
		if accountKey(row) == key {
			matches++
		} else if enabled && row.Code1 == key.Code1 && row.Status == "LAPORAN" {
			return fmt.Errorf("NOAC1 %s sudah dipakai akun laporan lain. Hilangkan centang akun tersebut terlebih dahulu.", key.Code1)
		}
	}
	if matches != 1 {
		return fmt.Errorf("Akun BANKLIST tidak ditemukan atau ambigu. Muat ulang Daftar Akun.")
	}
	wanted := ""
	if enabled {
		wanted = "LAPORAN"
	}
	after := bytes.Clone(before)
	changed := false
	for i := uint64(0); i < layout.count; i++ {
		start := layout.headerSize + int(i)*layout.recordSize
		row := after[start : start+layout.recordSize]
		if row[0] == '*' {
			continue
		}
		get := func(name string) string {
			f := layout.fields[name]
			return strings.TrimSpace(string(row[f.offset : f.offset+f.length]))
		}
		if (AccountKey{get("NOAC1"), get("NOAC2"), get("NOAC3")}) != key {
			continue
		}
		if get("STATUS") == wanted {
			return nil
		}
		f := layout.fields["STATUS"]
		copy(row[f.offset:f.offset+f.length], bytes.Repeat([]byte{' '}, f.length))
		copy(row[f.offset:f.offset+f.length], wanted)
		changed = true
	}
	if !changed {
		return fmt.Errorf("Akun tidak ditemukan.")
	}
	now := time.Now()
	after[1], after[2], after[3] = byte(now.Year()-1900), byte(now.Month()), byte(now.Day())
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err := replaceBankFile(path+".bak", before, info.Mode().Perm()); err != nil {
		return fmt.Errorf("Backup BANKLIST gagal; pilihan laporan belum disimpan: %w", err)
	}
	if err := replaceBankFile(path, after, info.Mode().Perm()); err != nil {
		return fmt.Errorf("Pilihan laporan gagal disimpan; file sebelumnya dipertahankan: %w", err)
	}
	return nil
}
