package fava

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverSlug(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/schweizerklub-norwegen/income_statement/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	slug, err := client.DiscoverSlug(context.Background())
	if err != nil {
		t.Fatalf("DiscoverSlug returned error: %v", err)
	}

	if slug != "schweizerklub-norwegen" {
		t.Fatalf("expected slug %q, got %q", "schweizerklub-norwegen", slug)
	}
}

const treeReportBody = `{"data":{"trees":[{
  "account":"Vermoegen",
  "balance":{},
  "balance_children":{"NOK":100.00},
  "children":[{
    "account":"Vermoegen:Bank:Giro",
    "balance":{"NOK":"100.00"},
    "balance_children":{"NOK":100.00},
    "has_txns":true
  }],
  "has_txns":false
}]},"mtime":"1234"}`

func TestTreeReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/ledgermainbeancount/income_statement/", http.StatusFound)
		case "/ledgermainbeancount/api/balance_sheet":
			if got := r.URL.Query().Get("time"); got != "2026" {
				t.Errorf("expected time=2026, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(treeReportBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	report, err := client.TreeReport(context.Background(), KindBalanceSheet, "2026")
	if err != nil {
		t.Fatalf("TreeReport returned error: %v", err)
	}

	if len(report.Trees) != 1 {
		t.Fatalf("expected 1 tree, got %d", len(report.Trees))
	}

	root := report.Trees[0]
	if root.Account != "Vermoegen" {
		t.Fatalf("expected account Vermoegen, got %q", root.Account)
	}
	if got := string(root.BalanceChildren["NOK"]); got != "100.00" {
		t.Fatalf("expected balance_children NOK 100.00, got %q", got)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	if got := string(root.Children[0].Balance["NOK"]); got != "100.00" {
		t.Fatalf("expected child balance NOK 100.00, got %q", got)
	}
}

func TestLedgerData(t *testing.T) {
	body := map[string]any{
		"data": map[string]any{
			"options": map[string]any{
				"title":       "Schweizerklub Norwegen",
				"name_assets": "Vermoegen",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/schweizerklub/income_statement/", http.StatusFound)
		case "/schweizerklub/api/ledger_data":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	data, err := client.LedgerData(context.Background())
	if err != nil {
		t.Fatalf("LedgerData returned error: %v", err)
	}

	if data.Options.Title != "Schweizerklub Norwegen" {
		t.Fatalf("expected title, got %q", data.Options.Title)
	}
	if data.Options.NameAssets != "Vermoegen" {
		t.Fatalf("expected name_assets Vermoegen, got %q", data.Options.NameAssets)
	}
}

func TestExtractSlug(t *testing.T) {
	for input, want := range map[string]string{
		"/schweizerklub-norwegen/income_statement/": "schweizerklub-norwegen",
		"/ledgermainbeancount/":                     "ledgermainbeancount",
		"/":                                         "",
		"http://localhost:5000/foo/bar":             "foo",
	} {
		if got := extractSlug(input); got != want {
			t.Errorf("extractSlug(%q) = %q, want %q", input, got, want)
		}
	}
}
