package report

import (
	"archive/zip"
	"path/filepath"
	"testing"
)

func TestWriteWorkbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "expenses.xlsx")
	rows := [][]string{
		{"Datum", "Buchungstext", "Betrag", "Waehrung"},
		{"2026-01-02", "Supermarket", "54.20", "EUR"},
	}

	if err := WriteWorkbook(path, "Expenses", rows); err != nil {
		t.Fatalf("WriteWorkbook returned error: %v", err)
	}

	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open xlsx as zip: %v", err)
	}
	defer reader.Close()

	requiredFiles := map[string]bool{
		"[Content_Types].xml":        false,
		"xl/workbook.xml":            false,
		"xl/styles.xml":              false,
		"xl/worksheets/sheet1.xml":   false,
		"xl/_rels/workbook.xml.rels": false,
	}

	for _, file := range reader.File {
		if _, ok := requiredFiles[file.Name]; ok {
			requiredFiles[file.Name] = true
		}
	}

	for name, found := range requiredFiles {
		if !found {
			t.Fatalf("missing workbook part %s", name)
		}
	}
}
