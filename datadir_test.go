package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run an actual test executable from a different working directory so neither
// cwd nor the development source tree can accidentally supply the data files.
func TestExecutableDataDirectory(t *testing.T) {
	if expected := os.Getenv("BUKUKAS_TEST_EXE_DIR"); expected != "" {
		service := NewCashService()
		if filepath.Clean(service.dataDir) != filepath.Clean(expected) {
			t.Fatalf("data directory %q must be executable directory %q", service.dataDir, expected)
		}
		rows, err := service.ListMasterAccounts()
		if os.Getenv("BUKUKAS_TEST_MISSING_DATA") == "1" {
			if err == nil {
				t.Fatal("missing data must not fall back to another folder")
			}
		} else if err != nil || len(rows.Rows) != 2 || rows.Rows[0].Code1 != "00300" {
			t.Fatalf("did not read file directly beside executable: %+v %v", rows, err)
		}
		if _, err := service.ListAccounts(); err == nil {
			t.Fatal("must not read BANKLIST from a dbf subfolder")
		}
		return
	}
	base := t.TempDir()
	appDir, cwd := filepath.Join(base, "app"), filepath.Join(base, "working")
	for _, dir := range []string{appDir, cwd, filepath.Join(appDir, "dbf"), filepath.Join(base, "dbf"), filepath.Join(cwd, "dbf")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(appDir, "buku-kas-path-test.exe")
	if err := os.WriteFile(app, data, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(appDir, "dbf"), filepath.Join(base, "dbf"), filepath.Join(cwd, "dbf")} {
		writeTestFile(t, filepath.Join(dir, "DAFACC.DBF"), reportMasterFixture([]AccountRow{{Code1: "99999", Code2: "000", Code3: "01"}}, []string{"0.00"}))
		writeTestFile(t, filepath.Join(dir, "BANKLIST.DBF"), bankFixture())
	}
	master := filepath.Join(appDir, "DAFACC.DBF")
	writeTestFile(t, master, accountFixture())
	for _, missing := range []bool{false, true} {
		if missing {
			if err := os.Remove(master); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(app, "-test.run=^TestExecutableDataDirectory$")
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(), "BUKUKAS_TEST_EXE_DIR="+appDir)
		if missing {
			cmd.Env = append(cmd.Env, "BUKUKAS_TEST_MISSING_DATA=1")
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("executable directory test (missing=%t): %v\n%s", missing, err, output)
		}
	}
}
