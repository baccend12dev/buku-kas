package main

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func accountFixture() []byte {
	fields := []cashField{{name: "NOAC1", kind: 'C', length: 5}, {name: "NOAC2", kind: 'C', length: 3}, {name: "NOAC3", kind: 'C', length: 2}, {name: "NAMA", kind: 'C', length: 52}}
	h, size := 161, 63
	data := make([]byte, h+size*3)
	data[0] = 3
	binary.LittleEndian.PutUint32(data[4:8], 3)
	binary.LittleEndian.PutUint16(data[8:10], uint16(h))
	binary.LittleEndian.PutUint16(data[10:12], uint16(size))
	for i, f := range fields {
		p := 32 + i*32
		copy(data[p:p+11], f.name)
		data[p+11] = f.kind
		data[p+16] = byte(f.length)
	}
	data[h-1] = 13
	for i := 0; i < 3; i++ {
		row := data[h+i*size : h+(i+1)*size]
		copy(row, strings.Repeat(" ", size))
		copy(row[1:6], []string{"00100", "00200", "00300"}[i])
		copy(row[6:9], "001")
		copy(row[9:11], "01")
		copy(row[11:], []string{"Pertama", "Dihapus", "Terakhir"}[i])
		if i == 1 {
			row[0] = '*'
		}
	}
	return data
}

func TestAccountsLastRecordFirstAndReadOnly(t *testing.T) {
	original := accountFixture()
	dir := t.TempDir()
	path := filepath.Join(dir, "DAFACC.DBF")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := (&CashService{dataDir: dir}).ListMasterAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || result.Rows[0].Code1 != "00300" || result.Rows[0].Name != "Terakhir" || result.Rows[1].Code1 != "00100" || result.Rows[0].Code2 != "001" || result.Rows[0].Code3 != "01" {
		t.Fatalf("Wrong account order or mapping: %+v", result)
	}
	after, _ := os.ReadFile(path)
	if sha256.Sum256(original) != sha256.Sum256(after) {
		t.Fatal("Account file modified")
	}
}

func TestAccountsInvalidAndEmpty(t *testing.T) {
	for name, alter := range map[string]func([]byte) []byte{
		"truncated":        func(b []byte) []byte { return b[:len(b)-1] },
		"wrong name type":  func(b []byte) []byte { b[32+3*32+11] = 'N'; return b },
		"unknown encoding": func(b []byte) []byte { b[161+11] = 255; return b },
		"invalid flag":     func(b []byte) []byte { b[161] = 0; return b },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readAccountRows(alter(accountFixture())); err == nil {
				t.Fatal("Expected error")
			}
		})
	}
	empty := accountFixture()[:161]
	binary.LittleEndian.PutUint32(empty[4:8], 0)
	result, err := readAccountRows(empty)
	if err != nil || result.Rows == nil || len(result.Rows) != 0 {
		t.Fatal("Empty account list should return an empty slice")
	}
	if _, err := (&CashService{}).ListAccounts(); err == nil {
		t.Fatal("Missing path should fail")
	}
}

func TestCashLastActiveRecordFirst(t *testing.T) {
	data := cashFixture(map[string]string{"NO_BKAS": "FIRST", "TGL_BKAS": "20261102", "JUMLAH": "1.00"}, map[string]string{"NO_BKAS": "OTHER", "TGL_BKAS": "20261103", "JUMLAH": "2.00"}, map[string]string{"NO_BKAS": "LAST", "TGL_BKAS": "20261102", "JUMLAH": "3.00"}, map[string]string{"NO_BKAS": "DELETED", "TGL_BKAS": "20261102", "JUMLAH": "4.00", "deleted": "yes"})
	for _, field := range []string{"KPD1", "KPK1"} {
		result, err := readCashRows(data, "20261102", field)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Rows) != 2 || result.Rows[0].Number != "LAST" || result.Rows[1].Number != "FIRST" {
			t.Fatal("Last active matching record must appear first")
		}
	}
}

func TestExistingAccountsReadOnly(t *testing.T) {
	path := filepath.Join("dbf", "DAFACC.DBF")
	before, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("Local DBF absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&CashService{dataDir: "dbf"}).ListMasterAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 518 {
		t.Fatalf("Expected 518 accounts, got %d", len(result.Rows))
	}
	size := int(binary.LittleEndian.Uint16(before[10:12]))
	h := int(binary.LittleEndian.Uint16(before[8:10]))
	n := int(binary.LittleEndian.Uint32(before[4:8]))
	for i := n - 1; i >= 0; i-- {
		p := h + i*size
		if before[p] == '*' {
			continue
		}
		if result.Rows[0].Code1 != strings.TrimSpace(string(before[p+1:p+6])) {
			t.Fatal("Last account not displayed first")
		}
		break
	}
	after, _ := os.ReadFile(path)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("Local account file modified")
	}
}
