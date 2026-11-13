package main

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cashFixture(rows ...map[string]string) []byte {
	fields := []cashField{{name: "NO_BKAS", kind: 'C', length: 7}, {name: "TGL_BKAS", kind: 'D', length: 8}, {name: "KPD1", kind: 'C', length: 5}, {name: "KPD2", kind: 'C', length: 5}, {name: "KPK1", kind: 'C', length: 5}, {name: "KPK2", kind: 'C', length: 5}, {name: "JUMLAH", kind: 'N', length: 15, decimals: 2}, {name: "KET", kind: 'M', length: 10}, {name: "BEBAN", kind: 'D', length: 8}, {name: "KA", kind: 'C', length: 1}}
	headerSize, recordSize := 33+32*len(fields), 70
	data := make([]byte, headerSize+recordSize*len(rows))
	data[0] = 0x83
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(rows)))
	binary.LittleEndian.PutUint16(data[8:10], uint16(headerSize))
	binary.LittleEndian.PutUint16(data[10:12], uint16(recordSize))
	for i, f := range fields {
		p := 32 + i*32
		copy(data[p:p+11], f.name)
		data[p+11] = f.kind
		data[p+16] = byte(f.length)
		data[p+17] = byte(f.decimals)
	}
	data[headerSize-1] = 0x0d
	for i, values := range rows {
		row := data[headerSize+i*recordSize : headerSize+(i+1)*recordSize]
		for p := range row {
			row[p] = ' '
		}
		if values["deleted"] == "yes" {
			row[0] = '*'
		}
		offset := 1
		for _, f := range fields {
			value := values[f.name]
			if f.kind == 'N' {
				value = strings.Repeat(" ", f.length-len(value)) + value
			}
			copy(row[offset:offset+f.length], value)
			offset += f.length
		}
	}
	return data
}

func TestCashInDateFilterAndPrecision(t *testing.T) {
	data := cashFixture(
		map[string]string{"NO_BKAS": "A1", "TGL_BKAS": "20261102", "JUMLAH": "999999999999.99", "KET": "12"},
		map[string]string{"NO_BKAS": "A2", "TGL_BKAS": "20261103", "JUMLAH": "100.00"},
		map[string]string{"NO_BKAS": "A3", "TGL_BKAS": "20261102", "JUMLAH": "200.00", "deleted": "yes"},
	)
	result, err := readCashRows(data, "20261102", "KPD1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0].Number != "A1" || result.Rows[0].Amount != "999999999999.99" || result.Rows[0].Date != "2026-11-02" || result.Rows[0].memoPointer != "12" {
		t.Fatalf("Unexpected result: %+v", result)
	}
	empty, err := readCashRows(data, "20261104", "KPD1")
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("Empty result must be a non-nil slice: %v", err)
	}
}

func TestCashInRejectsInvalidFiles(t *testing.T) {
	valid := func() []byte { return cashFixture(map[string]string{"TGL_BKAS": "20261102", "JUMLAH": "10.00"}) }
	cases := map[string]func() []byte{
		"truncated header": func() []byte { return []byte{0x83} },
		"truncated record": func() []byte { b := valid(); return b[:len(b)-1] },
		"bad field length": func() []byte { b := valid(); b[48] = 255; return b },
		"bad field type":   func() []byte { b := valid(); b[75] = 'C'; return b },
		"bad amount":       func() []byte { return cashFixture(map[string]string{"TGL_BKAS": "20261102", "JUMLAH": "1e5"}) },
		"bad memo pointer": func() []byte {
			return cashFixture(map[string]string{"TGL_BKAS": "20261102", "JUMLAH": "1.00", "KET": "bad"})
		},
		"unknown encoding": func() []byte {
			return cashFixture(map[string]string{"TGL_BKAS": "20261102", "JUMLAH": "1.00", "KPD1": "\xff"})
		},
	}
	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readCashRows(fixture(), "20261102", "KPD1"); err == nil {
				t.Fatal("Expected file validation error")
			}
		})
	}
}

func TestCashInServiceDateValidationAndReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "KAS_IN.DBF")
	original := cashFixture(map[string]string{"TGL_BKAS": "20261102", "JUMLAH": "10.00"})
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	service := &CashService{dataDir: dir}
	for _, date := range []string{"2026-02-30", "20261102", "", "2026-1-02", "../file"} {
		if _, err := service.ListCashIn(date); err == nil {
			t.Fatalf("Accepted invalid date %q", date)
		}
	}
	result, err := service.ListCashIn("2026-11-02")
	if err != nil || len(result.Rows) != 1 {
		t.Fatalf("Read failed: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(original) != sha256.Sum256(after) {
		t.Fatal("Source file changed")
	}
	if _, err := (&CashService{}).ListCashIn("2026-11-02"); err == nil {
		t.Fatal("Missing path should fail")
	}
}

func TestExistingCashInReadOnly(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("dbf", "KAS_IN.DBF"))
	if os.IsNotExist(err) {
		t.Skip("Local DBF not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&CashService{dataDir: "dbf"}).ListCashIn("2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 12 {
		t.Fatalf("Expected 12 active records for 2026-11-02, got %d", len(result.Rows))
	}
	for _, row := range result.Rows {
		if row.Date != "2026-11-02" {
			t.Fatal("Wrong date returned")
		}
	}
	after, err := os.ReadFile(filepath.Join("dbf", "KAS_IN.DBF"))
	if err != nil || sha256.Sum256(original) != sha256.Sum256(after) {
		t.Fatal("Local DBF changed")
	}
}

func TestCashMemo(t *testing.T) {
	for _, marker := range []string{"\x1a", "\x1a\x1a"} {
		text := "\r\n" + strings.Repeat("Keterangan panjang. ", 40) + "\r\nBaris kedua\r\n"
		memo := append(make([]byte, 512), []byte(text+marker+"stale padding")...)
		got, err := readCashMemo(memo, "1")
		want := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
		if err != nil || got != want {
			t.Fatalf("Multi-block memo mismatch: %v", err)
		}
	}
	for name, tc := range map[string]struct {
		memo    []byte
		pointer string
	}{
		"header reference":   {make([]byte, 512), "-1"},
		"invalid pointer":    {make([]byte, 512), "bad"},
		"overflow":           {make([]byte, 512), "999999999999999999999999999999"},
		"out of range":       {make([]byte, 512), "1"},
		"missing terminator": {append(make([]byte, 512), []byte("unfinished")...), "1"},
		"unknown encoding":   {append(make([]byte, 512), []byte{0xff, 0x1a}...), "1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readCashMemo(tc.memo, tc.pointer); err == nil {
				t.Fatal("Expected memo error")
			}
		})
	}
	for _, pointer := range []string{"", "0", "0000"} {
		if got, err := readCashMemo(nil, pointer); err != nil || got != "" {
			t.Fatal("Empty memo should remain empty")
		}
	}
}

func TestCashServiceMemoAndAccountMapping(t *testing.T) {
	dir := t.TempDir()
	service := &CashService{dataDir: dir}
	dbf := cashFixture(map[string]string{"NO_BKAS": "TEST001", "TGL_BKAS": "20261102", "KPD1": "00123", "KPK1": "00456", "JUMLAH": "100.25", "KET": "1"})
	memo := append(make([]byte, 512), []byte("\r\nKeterangan uji\r\nbaris kedua\x1a")...)
	for _, table := range []string{"KAS_IN", "KAS_OUT"} {
		if err := os.WriteFile(filepath.Join(dir, table+".DBF"), dbf, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, table+".DBT"), memo, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, account string
		load          func(string) (CashResult, error)
	}{
		{"KAS_IN", "00123", service.ListCashIn}, {"KAS_OUT", "00456", service.ListCashOut},
	} {
		result, err := tc.load("2026-11-02")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Rows) != 1 || result.Rows[0].Account != tc.account || result.Rows[0].Description != "Keterangan uji\nbaris kedua" || result.MemoUnavailable {
			t.Fatalf("Wrong account or memo for %s", tc.name)
		}
		for _, suffix := range []string{".DBF", ".DBT"} {
			after, err := os.ReadFile(filepath.Join(dir, tc.name+suffix))
			want := dbf
			if suffix == ".DBT" {
				want = memo
			}
			if err != nil || sha256.Sum256(after) != sha256.Sum256(want) {
				t.Fatal("DBF/DBT source modified")
			}
		}
	}
	if err := os.Remove(filepath.Join(dir, "KAS_OUT.DBT")); err != nil {
		t.Fatal(err)
	}
	missing, err := service.ListCashOut("2026-11-02")
	if err != nil || !missing.MemoUnavailable || len(missing.Rows) != 1 {
		t.Fatal("Missing memo should show a warning without discarding rows")
	}
}

func TestExistingCashMemosReadOnly(t *testing.T) {
	service := &CashService{dataDir: "dbf"}
	for _, tc := range []struct {
		table string
		load  func(string) (CashResult, error)
	}{
		{"KAS_IN", service.ListCashIn}, {"KAS_OUT", service.ListCashOut},
	} {
		originalDBF, err := os.ReadFile(filepath.Join("dbf", tc.table+".DBF"))
		if os.IsNotExist(err) {
			t.Skip("Local data absent")
		}
		if err != nil {
			t.Fatal(err)
		}
		originalDBT, err := os.ReadFile(filepath.Join("dbf", tc.table+".DBT"))
		if os.IsNotExist(err) {
			t.Skip("Local memo absent")
		}
		if err != nil {
			t.Fatal(err)
		}
		// Read every active date to include final memo blocks and every description.
		h := int(binary.LittleEndian.Uint16(originalDBF[8:10]))
		size := int(binary.LittleEndian.Uint16(originalDBF[10:12]))
		count := int(binary.LittleEndian.Uint32(originalDBF[4:8]))
		dates := map[string]bool{}
		for i := 0; i < count; i++ {
			row := originalDBF[h+i*size : h+(i+1)*size]
			if row[0] == '*' {
				continue
			}
			raw := string(row[8:16])
			dates[raw[:4]+"-"+raw[4:6]+"-"+raw[6:]] = true
		}
		for date := range dates {
			result, err := tc.load(date)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.table, date, err)
			}
			if result.MemoUnavailable {
				t.Fatal("Existing DBT was not loaded")
			}
			for _, row := range result.Rows {
				if row.Description == "" {
					t.Fatalf("Empty memo for %s on %s", tc.table, date)
				}
			}
		}
		afterDBF, _ := os.ReadFile(filepath.Join("dbf", tc.table+".DBF"))
		afterDBT, _ := os.ReadFile(filepath.Join("dbf", tc.table+".DBT"))
		if sha256.Sum256(originalDBF) != sha256.Sum256(afterDBF) || sha256.Sum256(originalDBT) != sha256.Sum256(afterDBT) {
			t.Fatal("Local source modified")
		}
	}
}
