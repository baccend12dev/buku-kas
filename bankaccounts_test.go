package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func bankFixture(rows ...AccountRow) []byte {
	fields := []cashField{{name: "NOAC1", kind: 'C', length: 5}, {name: "NOAC2", kind: 'C', length: 3}, {name: "NOAC3", kind: 'C', length: 2}, {name: "NAMA", kind: 'C', length: 52}, {name: "STATUS", kind: 'C', length: 10}}
	data := make([]byte, 193+73*len(rows)+1)
	data[0] = 3
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(rows)))
	binary.LittleEndian.PutUint16(data[8:10], 193)
	binary.LittleEndian.PutUint16(data[10:12], 73)
	for i, f := range fields {
		p := 32 + i*32
		copy(data[p:p+11], f.name)
		data[p+11], data[p+16] = f.kind, byte(f.length)
	}
	data[192] = 13
	for i, row := range rows {
		record := data[193+i*73 : 193+(i+1)*73]
		copy(record, bytes.Repeat([]byte{' '}, 73))
		copy(record[1:6], row.Code1)
		copy(record[6:9], row.Code2)
		copy(record[9:11], row.Code3)
		copy(record[11:63], row.Name)
		copy(record[63:73], row.Status)
	}
	data[len(data)-1] = 0x1a
	return data
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bankServiceFixture(t *testing.T) (*CashService, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	bank := bankFixture()
	writeTestFile(t, filepath.Join(dir, "BANKLIST.DBF"), bank)
	writeTestFile(t, filepath.Join(dir, "DAFACC.DBF"), accountFixture())
	return &CashService{dataDir: dir}, filepath.Join(dir, "BANKLIST.DBF"), bank
}

func TestBankAddBatchBackupAndPersistence(t *testing.T) {
	s, path, original := bankServiceFixture(t)
	master := readTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF"))
	empty, err := s.ListAccounts()
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	keys := []AccountKey{{"00100", "001", "01"}, {"00300", "001", "01"}}
	result, err := s.AddBankAccounts(keys)
	if err != nil || result.Added != 2 || result.Skipped != 0 {
		t.Fatalf("add: %+v %v", result, err)
	}
	if !bytes.Equal(readTestFile(t, path+".bak"), original) {
		t.Fatal("backup must contain original bytes")
	}
	if !bytes.Equal(readTestFile(t, filepath.Join(s.dataDir, "DAFACC.DBF")), master) {
		t.Fatal("master changed")
	}
	data := readTestFile(t, path)
	if data[len(data)-1] != 0x1a || binary.LittleEndian.Uint32(data[4:8]) != 2 {
		t.Fatal("invalid header or EOF")
	}
	now := time.Now()
	if data[1] != byte(now.Year()-1900) || data[2] != byte(now.Month()) || data[3] != byte(now.Day()) {
		t.Fatal("header date not updated")
	}
	for i := 0; i < 2; i++ {
		if string(data[193+i*73+63:193+(i+1)*73]) != "          " {
			t.Fatal("status must be blank")
		}
	}
	persisted, err := (&CashService{dataDir: s.dataDir}).ListAccounts()
	if err != nil || len(persisted.Rows) != 2 || persisted.Rows[0].Code1 != "00300" || persisted.Rows[1].Code1 != "00100" || persisted.Rows[0].Name != "Terakhir" {
		t.Fatalf("persistence: %+v %v", persisted, err)
	}
	result, err = s.AddBankAccounts(keys)
	if err != nil || result.Added != 0 || result.Skipped != 2 {
		t.Fatalf("retry: %+v %v", result, err)
	}
	if !bytes.Equal(data, readTestFile(t, path)) || !bytes.Equal(original, readTestFile(t, path+".bak")) {
		t.Fatal("idempotent retry changed files")
	}
}

func TestBankValidationIsAllOrNothing(t *testing.T) {
	for _, invalid := range []AccountKey{{"99999", "001", "01"}, {"00200", "001", "01"}, {"00100", "", ""}} {
		s, path, original := bankServiceFixture(t)
		_, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}, invalid})
		if err == nil {
			t.Fatal("invalid or deleted account accepted")
		}
		if !bytes.Equal(original, readTestFile(t, path)) {
			t.Fatal("partial batch written")
		}
		if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
			t.Fatal("validation must precede backup")
		}
	}
	s, _, _ := bankServiceFixture(t)
	if _, err := s.AddBankAccounts(nil); err == nil {
		t.Fatal("empty selection accepted")
	}
}

func TestBankFullKeyAndAmbiguousMaster(t *testing.T) {
	s, path, original := bankServiceFixture(t)
	masterPath := filepath.Join(s.dataDir, "DAFACC.DBF")
	master := readTestFile(t, masterPath)
	copy(master[161+2*63+1:161+2*63+6], "00100")
	copy(master[161+2*63+6:161+2*63+9], "002")
	writeTestFile(t, masterPath, master)
	keys := []AccountKey{{"00100", "001", "01"}, {"00100", "002", "01"}}
	result, err := s.AddBankAccounts(append(keys, keys[0]))
	if err != nil || result.Added != 2 || result.Skipped != 1 {
		t.Fatalf("full identity: %+v %v", result, err)
	}
	writeTestFile(t, path, original)
	copy(master[161+2*63+6:161+2*63+9], "001")
	writeTestFile(t, masterPath, master)
	if _, err := s.AddBankAccounts(keys[:1]); err == nil {
		t.Fatal("ambiguous master accepted")
	}
	if !bytes.Equal(original, readTestFile(t, path)) {
		t.Fatal("ambiguous write changed bank")
	}
}

func TestBankPreservesRecordsAndIgnoresStatus(t *testing.T) {
	s, path, _ := bankServiceFixture(t)
	before := bankFixture(AccountRow{Code1: "00100", Code2: "001", Code3: "01", Name: "Nama sebelumnya"}, AccountRow{Code1: "00300", Code2: "001", Code3: "01", Name: "Dihapus"})
	copy(before[193+63:193+73], "NONAKTIF  ")
	before[193+73] = '*'
	writeTestFile(t, path, before)
	listed, err := s.ListAccounts()
	if err != nil || len(listed.Rows) != 1 {
		t.Fatalf("status is not a filter: %+v %v", listed, err)
	}
	result, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}, {"00300", "001", "01"}})
	if err != nil || result.Added != 1 || result.Skipped != 1 {
		t.Fatalf("deleted handling: %+v %v", result, err)
	}
	after := readTestFile(t, path)
	if !bytes.Equal(before[193:len(before)-1], after[193:len(before)-1]) {
		t.Fatal("existing records changed")
	}
}

func TestBankMissingMalformedAndBackupFailure(t *testing.T) {
	for _, alter := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:192] },
		func(b []byte) []byte { b[32+4*32+16] = 9; return b },
		func(b []byte) []byte { b[len(b)-1] = 0; return b },
	} {
		s, path, before := bankServiceFixture(t)
		broken := alter(before)
		writeTestFile(t, path, broken)
		if _, err := s.ListAccounts(); err == nil {
			t.Fatal("malformed list accepted")
		}
		if _, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}}); err == nil {
			t.Fatal("malformed write accepted")
		}
		if !bytes.Equal(broken, readTestFile(t, path)) {
			t.Fatal("broken original changed")
		}
	}
	s, path, original := bankServiceFixture(t)
	if err := os.Mkdir(path+".bak", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}}); err == nil {
		t.Fatal("backup failure ignored")
	}
	if !bytes.Equal(original, readTestFile(t, path)) {
		t.Fatal("backup failure changed target")
	}
	s.dataDir = t.TempDir()
	if _, err := s.ListAccounts(); err == nil {
		t.Fatal("missing bank accepted")
	}
	if _, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}}); err == nil {
		t.Fatal("missing bank recreated")
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "BANKLIST.DBF")); !os.IsNotExist(err) {
		t.Fatal("bank must not be created")
	}
}

// A helper process proves locking also works between separate application instances.
func TestBankLockProcess(t *testing.T) {
	if path := os.Getenv("BUKUKAS_TEST_LOCK"); path != "" {
		unlock, err := lockBankFile(path)
		if err != nil {
			os.Exit(0)
		}
		unlock()
		os.Exit(5)
	}
	s, path, before := bankServiceFixture(t)
	unlock, err := lockBankFile(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		unlock()
		t.Fatal(err)
	}
	command := func() *exec.Cmd {
		cmd := exec.Command(exe, "-test.run=^TestBankLockProcess$")
		cmd.Env = append(os.Environ(), "BUKUKAS_TEST_LOCK="+path+".lock")
		return cmd
	}
	if out, err := command().CombinedOutput(); err != nil {
		unlock()
		t.Fatalf("other process acquired lock: %v %s", err, out)
	}
	if _, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}}); err == nil {
		unlock()
		t.Fatal("write ignored lock")
	}
	unlock()
	if !bytes.Equal(before, readTestFile(t, path)) {
		t.Fatal("locked write changed bank")
	}
	err = command().Run()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 5 {
		t.Fatalf("lock not released: %v", err)
	}
	if _, err := s.AddBankAccounts([]AccountKey{{"00100", "001", "01"}}); err != nil {
		t.Fatal(err)
	}
}

func TestBankConcurrentAddsKeepBothAccounts(t *testing.T) {
	s, _, _ := bankServiceFixture(t)
	var group sync.WaitGroup
	for _, key := range []AccountKey{{"00100", "001", "01"}, {"00300", "001", "01"}} {
		group.Add(1)
		go func(key AccountKey) {
			defer group.Done()
			deadline := time.Now().Add(5 * time.Second)
			for {
				_, err := (&CashService{dataDir: s.dataDir}).AddBankAccounts([]AccountKey{key})
				if err == nil {
					return
				}
				if time.Now().After(deadline) {
					t.Error(err)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(key)
	}
	group.Wait()
	result, err := s.ListAccounts()
	if err != nil || len(result.Rows) != 2 {
		t.Fatalf("lost concurrent update: %+v %v", result, err)
	}
}
