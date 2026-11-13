package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/xuri/excelize/v2"
)

type ReportExportResult struct {
	Cancelled bool   `json:"cancelled"`
	Path      string `json:"path"`
}

func (s *CashService) ExportCashReport(date, revision string) (ReportExportResult, error) {
	return s.exportCashReport(date, revision, func(filename string) (string, error) {
		app := application.Get()
		if app == nil {
			return "", fmt.Errorf("Dialog Simpan hanya tersedia di aplikasi desktop.")
		}
		owner := app.Window.Current()
		if owner == nil {
			return "", fmt.Errorf("Dialog Simpan hanya tersedia di aplikasi desktop.")
		}
		return app.Dialog.SaveFile().SetFilename(filename).AddFilter("Excel Workbook", "*.xlsx").AllowsOtherFileTypes(false).AttachToWindow(owner).PromptForSingleSelection()
	})
}

func (s *CashService) exportCashReport(date, revision string, choose func(string) (string, error)) (ReportExportResult, error) {
	report, err := s.GetCashReport(date)
	if err != nil {
		return ReportExportResult{}, err
	}
	if len(report.Columns) == 0 {
		return ReportExportResult{}, fmt.Errorf("Pilih akun laporan di Daftar Akun terlebih dahulu.")
	}
	if revision == "" || revision != report.Revision {
		return ReportExportResult{}, fmt.Errorf("Data laporan berubah. Muat ulang laporan sebelum ekspor.")
	}
	if err := validateReportExcelPrecision(report); err != nil {
		return ReportExportResult{}, err
	}
	path, err := choose("Laporan_Kas_" + date + ".xlsx")
	if err != nil {
		return ReportExportResult{}, fmt.Errorf("Dialog Simpan gagal: %w", err)
	}
	if path == "" {
		return ReportExportResult{Cancelled: true}, nil
	}
	if filepath.Ext(path) == "" {
		path += ".xlsx"
	}
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return ReportExportResult{}, fmt.Errorf("Gunakan nama file dengan ekstensi .xlsx.")
	}
	// A save dialog can remain open while source data changes.
	report, err = s.GetCashReport(date)
	if err != nil {
		return ReportExportResult{}, err
	}
	if report.Revision != revision {
		return ReportExportResult{}, fmt.Errorf("Data laporan berubah saat dialog Simpan terbuka. Muat ulang laporan sebelum ekspor.")
	}
	book, err := buildReportWorkbook(report)
	if err != nil {
		return ReportExportResult{}, err
	}
	defer book.Close()
	if err := saveReportWorkbook(book, path); err != nil {
		return ReportExportResult{}, fmt.Errorf("Laporan gagal disimpan; file tujuan sebelumnya dipertahankan: %w", err)
	}
	return ReportExportResult{Path: path}, nil
}

func validateReportExcelPrecision(report CashReport) error {
	for _, row := range report.Rows {
		for _, value := range row.Amounts {
			if value == "" {
				continue
			}
			if _, err := parseCents(value); err != nil {
				return err
			}
			digits := strings.Trim(strings.ReplaceAll(strings.TrimPrefix(value, "-"), ".", ""), "0")
			if len(digits) > 15 {
				return fmt.Errorf("Nominal pada %s melebihi ketelitian 15 digit Excel. Ekspor dibatalkan agar angka tidak berubah.", row.Description)
			}
		}
	}
	return nil
}

func buildReportWorkbook(report CashReport) (*excelize.File, error) {
	if len(report.Columns) == 0 {
		return nil, fmt.Errorf("Belum ada akun laporan.")
	}
	if err := validateReportExcelPrecision(report); err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	const sheet = "Laporan Kas"
	fail := func(err error) (*excelize.File, error) { book.Close(); return nil, err }
	if err := book.SetSheetName("Sheet1", sheet); err != nil {
		return fail(err)
	}
	lastCol, err := excelize.ColumnNumberToName(3 + len(report.Columns))
	if err != nil {
		return fail(err)
	}
	if len(report.Rows)+len(report.Warnings)+7 > 1048576 {
		return fail(fmt.Errorf("Laporan melampaui jumlah baris Excel."))
	}
	numberFormat := "#,##0.00"
	styles := map[string]int{}
	for kind, color := range map[string]string{"transaction": "FFFFFF", "opening": "F2F2F2", "section": "F2F2F2", "total": "FFFFCC", "accumulation": "D9E2F3", "closing": "E2EFDA", "header": "D9D9D9"} {
		style := &excelize.Style{Font: &excelize.Font{Family: "Arial", Size: 12, Bold: kind != "transaction"}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{color}}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}, Border: []excelize.Border{{Type: "bottom", Color: "DDE3DF", Style: 1}}, CustomNumFmt: &numberFormat}
		if kind == "header" {
			style.Alignment.Horizontal = "center"
		}
		id, err := book.NewStyle(style)
		if err != nil {
			return fail(err)
		}
		styles[kind] = id
	}
	titleStyle, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "Arial", Size: 14, Bold: true}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	if err != nil {
		return fail(err)
	}
	date, err := time.Parse("2006-01-02", report.Date)
	if err != nil || date.Year() < 1 {
		return fail(fmt.Errorf("Tanggal laporan tidak valid."))
	}
	months := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	for index, value := range []string{report.Title, fmt.Sprintf("Tanggal: %d %s %d", date.Day(), months[int(date.Month())-1], date.Year())} {
		row := index + 1
		if err := book.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("%s%d", lastCol, row)); err != nil {
			return fail(err)
		}
		if err := book.SetCellStr(sheet, fmt.Sprintf("A%d", row), value); err != nil {
			return fail(err)
		}
		if err := book.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("%s%d", lastCol, row), titleStyle); err != nil {
			return fail(err)
		}
		if err := book.SetRowHeight(sheet, row, 24); err != nil {
			return fail(err)
		}
	}
	headers := []string{"URAIAN", "NO BKAS", "NO."}
	for _, column := range report.Columns {
		headers = append(headers, column.Name)
	}
	for i, value := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		if err := book.SetCellStr(sheet, cell, value); err != nil {
			return fail(err)
		}
		second := ""
		if i == 2 {
			second = "CHECK/GIRO"
		} else if i >= 3 {
			second = report.Columns[i-3].Code
		}
		cell, _ = excelize.CoordinatesToCellName(i+1, 5)
		if err := book.SetCellStr(sheet, cell, second); err != nil {
			return fail(err)
		}
	}
	if err := book.SetCellStyle(sheet, "A4", lastCol+"5", styles["header"]); err != nil {
		return fail(err)
	}
	if err := book.SetRowHeight(sheet, 4, 44); err != nil {
		return fail(err)
	}
	if err := book.SetRowHeight(sheet, 5, 22); err != nil {
		return fail(err)
	}
	for i, row := range report.Rows {
		n := i + 6
		if row.Kind == "spacer" {
			if err := book.SetRowHeight(sheet, n, 12); err != nil {
				return fail(err)
			}
			continue
		}
		for j, value := range []string{row.Description, row.Number, row.Check} {
			cell, _ := excelize.CoordinatesToCellName(j+1, n)
			if err := book.SetCellStr(sheet, cell, value); err != nil {
				return fail(err)
			}
		}
		for j, value := range row.Amounts {
			if value == "" {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(j+4, n)
			// Numeric XML from an exact decimal string; do not convert money to float64.
			if err := book.SetCellDefault(sheet, cell, value); err != nil {
				return fail(err)
			}
		}
		style, ok := styles[row.Kind]
		if !ok {
			return fail(fmt.Errorf("Jenis baris laporan tidak dikenal: %s", row.Kind))
		}
		if err := book.SetCellStyle(sheet, fmt.Sprintf("A%d", n), fmt.Sprintf("%s%d", lastCol, n), style); err != nil {
			return fail(err)
		}
		lines := strings.Count(row.Description, "\n") + 1 + (len([]rune(row.Description)) / 44)
		if lines < 1 {
			lines = 1
		}
		if err := book.SetRowHeight(sheet, n, float64(20*lines)); err != nil {
			return fail(err)
		}
	}
	lastRow := len(report.Rows) + 5
	for _, warning := range report.Warnings {
		lastRow++
		cell := fmt.Sprintf("A%d", lastRow)
		if err := book.MergeCell(sheet, cell, fmt.Sprintf("%s%d", lastCol, lastRow)); err != nil {
			return fail(err)
		}
		if err := book.SetCellStr(sheet, cell, warning); err != nil {
			return fail(err)
		}
		if err := book.SetRowHeight(sheet, lastRow, 32); err != nil {
			return fail(err)
		}
		if err := book.SetCellStyle(sheet, cell, fmt.Sprintf("%s%d", lastCol, lastRow), styles["transaction"]); err != nil {
			return fail(err)
		}
	}
	for _, width := range []struct {
		from, to string
		size     float64
	}{{"A", "A", 45}, {"B", "B", 14}, {"C", "C", 16}, {"D", lastCol, 27}} {
		if err := book.SetColWidth(sheet, width.from, width.to, width.size); err != nil {
			return fail(err)
		}
	}
	if err := book.SetColVisible(sheet, "C", false); err != nil {
		return fail(err)
	}
	if err := book.SetPanes(sheet, &excelize.Panes{Freeze: true, XSplit: 2, YSplit: 5, TopLeftCell: "D6", ActivePane: "bottomRight"}); err != nil {
		return fail(err)
	}
	grid := false
	if err := book.SetSheetView(sheet, 0, &excelize.ViewOptions{ShowGridLines: &grid}); err != nil {
		return fail(err)
	}
	if err := book.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", RefersTo: fmt.Sprintf("'%s'!$A$1:$%s$%d", sheet, lastCol, lastRow), Scope: sheet}); err != nil {
		return fail(err)
	}
	if err := book.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", RefersTo: "'Laporan Kas'!$1:$5", Scope: sheet}); err != nil {
		return fail(err)
	}
	size, width, height, orientation := 9, 1, 0, "portrait"
	if err := book.SetPageLayout(sheet, &excelize.PageLayoutOptions{Size: &size, Orientation: &orientation, FitToWidth: &width, FitToHeight: &height}); err != nil {
		return fail(err)
	}
	return book, nil
}

func saveReportWorkbook(book *excelize.File, path string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".cash-report-*.xlsx")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := book.Write(f); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return replaceFile(f.Name(), path)
}
