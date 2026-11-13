package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

type AccountKey struct {
	Code1 string `json:"code1"`
	Code2 string `json:"code2"`
	Code3 string `json:"code3"`
}

type AddAccountsResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

func accountKey(row AccountRow) AccountKey {
	return AccountKey{row.Code1, row.Code2, row.Code3}
}

// BANKLIST has no memo or index; reject other layouts before writing any bytes.
func bankLayout(data []byte) (dbfLayout, error) {
	layout, err := readDBFLayout(data)
	if err != nil {
		return dbfLayout{}, err
	}
	if data[0] != 3 || data[28] != 0 || len(layout.fields) != 5 || layout.recordSize != 73 {
		return dbfLayout{}, fmt.Errorf("struktur BANKLIST tidak didukung")
	}
	for name, size := range map[string]int{"NOAC1": 5, "NOAC2": 3, "NOAC3": 2, "NAMA": 52, "STATUS": 10} {
		field, ok := layout.fields[name]
		if !ok || field.kind != 'C' || field.length != size || field.decimals != 0 {
			return dbfLayout{}, fmt.Errorf("struktur field %s tidak didukung", name)
		}
	}
	end := layout.headerSize + int(layout.count)*layout.recordSize
	if len(data) != end && !(len(data) == end+1 && data[end] == 0x1a) {
		return dbfLayout{}, fmt.Errorf("penanda akhir BANKLIST tidak valid")
	}
	return layout, nil
}

func (s *CashService) AddBankAccounts(keys []AccountKey) (AddAccountsResult, error) {
	if s.dataDir == "" {
		return AddAccountsResult{}, fmt.Errorf("Lokasi aplikasi tidak dapat ditentukan. File DBF dan DBT harus berada di folder yang sama dengan executable.")
	}
	if len(keys) == 0 {
		return AddAccountsResult{}, fmt.Errorf("Pilih setidaknya satu akun.")
	}
	path := filepath.Join(s.dataDir, "BANKLIST.DBF")
	unlock, err := lockBankFile(path + ".lock")
	if err != nil {
		return AddAccountsResult{}, fmt.Errorf("BANKLIST sedang digunakan atau tidak dapat dikunci. Coba lagi setelah proses lain selesai: %w", err)
	}
	defer unlock()
	before, err := os.ReadFile(path)
	if err != nil {
		return AddAccountsResult{}, fmt.Errorf("BANKLIST.DBF tidak dapat dibaca. Periksa keberadaan file dan izin akses: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return AddAccountsResult{}, err
	}
	layout, err := bankLayout(before)
	if err != nil {
		return AddAccountsResult{}, err
	}
	bank, err := readAccountRows(before)
	if err != nil {
		return AddAccountsResult{}, fmt.Errorf("Gagal membaca BANKLIST.DBF: %w", err)
	}
	master, err := s.ListMasterAccounts()
	if err != nil {
		return AddAccountsResult{}, err
	}
	lookup := map[AccountKey][]AccountRow{}
	for _, row := range master.Rows {
		key := accountKey(row)
		lookup[key] = append(lookup[key], row)
	}
	existing := map[AccountKey]bool{}
	for _, row := range bank.Rows {
		existing[accountKey(row)] = true
	}
	result := AddAccountsResult{}
	var additions []AccountRow
	for _, key := range keys {
		matches := lookup[key]
		if len(matches) != 1 {
			return AddAccountsResult{}, fmt.Errorf("Akun %s/%s/%s tidak ditemukan atau ambigu di DAFACC. Muat ulang pilihan akun.", key.Code1, key.Code2, key.Code3)
		}
		if existing[key] {
			result.Skipped++
			continue
		}
		existing[key] = true
		additions = append(additions, matches[0])
	}
	if len(additions) == 0 {
		return result, nil
	}
	if layout.count+uint64(len(additions)) > math.MaxUint32 {
		return AddAccountsResult{}, fmt.Errorf("BANKLIST sudah mencapai batas jumlah record.")
	}
	end := layout.headerSize + int(layout.count)*layout.recordSize
	after := bytes.Clone(before[:end])
	for _, row := range additions {
		record := bytes.Repeat([]byte{' '}, layout.recordSize)
		for name, value := range map[string]string{"NOAC1": row.Code1, "NOAC2": row.Code2, "NOAC3": row.Code3, "NAMA": row.Name} {
			field := layout.fields[name]
			copy(record[field.offset:field.offset+field.length], value)
		}
		after = append(after, record...)
	}
	after = append(after, 0x1a)
	now := time.Now()
	after[1], after[2], after[3] = byte(now.Year()-1900), byte(now.Month()), byte(now.Day())
	binary.LittleEndian.PutUint32(after[4:8], uint32(layout.count)+uint32(len(additions)))
	if err := replaceBankFile(path+".bak", before, info.Mode().Perm()); err != nil {
		return AddAccountsResult{}, fmt.Errorf("Backup BANKLIST gagal; akun belum disimpan: %w", err)
	}
	if err := replaceBankFile(path, after, info.Mode().Perm()); err != nil {
		return AddAccountsResult{}, fmt.Errorf("BANKLIST gagal disimpan; file sebelumnya dipertahankan. Periksa izin akses atau file yang terkunci: %w", err)
	}
	result.Added = len(additions)
	return result, nil
}

// Stage and flush in the same directory, then replace without truncating the target.
func replaceBankFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".banklist-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return replaceFile(temp, path)
}
