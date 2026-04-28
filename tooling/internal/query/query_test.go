package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeParamsMonthBounds(t *testing.T) {
	params, err := NormalizeParams("2026-01", "2026-02")
	if err != nil {
		t.Fatalf("NormalizeParams returned error: %v", err)
	}

	if params.From != "2026-01-01" {
		t.Fatalf("unexpected from value: %s", params.From)
	}

	if params.To != "2026-02-28" {
		t.Fatalf("unexpected to value: %s", params.To)
	}
}

func TestNormalizeParamsRejectsReverseRange(t *testing.T) {
	_, err := NormalizeParams("2026-03-01", "2026-02-01")
	if err == nil {
		t.Fatal("expected error for reverse range")
	}
}

func TestRenderTemplateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.tmpl.bql")
	content := "SELECT * FROM postings WHERE 1 = 1{{if .From}} AND date >= {{.From}}{{end}}{{if .To}} AND date <= {{.To}}{{end}};"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	query, err := RenderTemplateFile(path, Params{
		From: "2026-01-01",
		To:   "2026-12-31",
	})
	if err != nil {
		t.Fatalf("RenderTemplateFile returned error: %v", err)
	}

	if !strings.Contains(query, "date >= 2026-01-01") {
		t.Fatalf("rendered query missing from bound: %s", query)
	}

	if !strings.Contains(query, "date <= 2026-12-31") {
		t.Fatalf("rendered query missing to bound: %s", query)
	}
}
