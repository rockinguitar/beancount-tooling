package report

import (
	"strings"
	"testing"

	"github.com/rockinguitar/beancount-tooling/tooling/internal/fava"
)

func sampleBalance() *fava.TreeReport {
	return &fava.TreeReport{
		Trees: []fava.TreeNode{
			{
				Account:         "Vermoegen",
				BalanceChildren: fava.Amount{"NOK": "1000.00"},
				Children: []fava.TreeNode{
					{Account: "Vermoegen:Bank:Giro", Balance: fava.Amount{"NOK": "600.00"}, BalanceChildren: fava.Amount{"NOK": "600.00"}},
					{Account: "Vermoegen:Bank:Fest", Balance: fava.Amount{"NOK": "400.00"}, BalanceChildren: fava.Amount{"NOK": "400.00"}},
				},
			},
			{
				Account:         "Verbindlichkeiten",
				BalanceChildren: fava.Amount{"NOK": "-200.00"},
				Children: []fava.TreeNode{
					{Account: "Verbindlichkeiten:Vorauszahlungen", Balance: fava.Amount{"NOK": "-200.00"}, BalanceChildren: fava.Amount{"NOK": "-200.00"}},
				},
			},
			{
				Account:         "Eigenkapital",
				BalanceChildren: fava.Amount{"NOK": "-800.00"},
				Children: []fava.TreeNode{
					{Account: "Eigenkapital:Anfangsbestand", Balance: fava.Amount{"NOK": "-800.00"}, BalanceChildren: fava.Amount{"NOK": "-800.00"}},
				},
			},
		},
	}
}

func sampleIncome() *fava.TreeReport {
	return &fava.TreeReport{
		Trees: []fava.TreeNode{
			{
				Account:         "Einnahmen",
				BalanceChildren: fava.Amount{"NOK": "-5000.00"},
				Children: []fava.TreeNode{
					{Account: "Einnahmen:Mitgliedsbeitraege", Balance: fava.Amount{"NOK": "-5000.00"}, BalanceChildren: fava.Amount{"NOK": "-5000.00"}},
				},
			},
			{Account: "Net Profit", BalanceChildren: fava.Amount{"NOK": "-3000.00"}},
			{
				Account:         "Ausgaben",
				BalanceChildren: fava.Amount{"NOK": "2000.00"},
				Children: []fava.TreeNode{
					{Account: "Ausgaben:Betrieb", Balance: fava.Amount{"NOK": "2000.00"}, BalanceChildren: fava.Amount{"NOK": "2000.00"}},
				},
			},
		},
	}
}

func TestRenderStatementsHTML(t *testing.T) {
	html, err := RenderStatementsHTML(StatementsDoc{
		Title:   "Schweizerklub Norwegen",
		Year:    "2026",
		Balance: sampleBalance(),
		Income:  sampleIncome(),
	})
	if err != nil {
		t.Fatalf("RenderStatementsHTML returned error: %v", err)
	}

	for _, want := range []string{
		"Schweizerklub Norwegen",
		"Bilanz per 31.12.2026",
		"Aktiven",
		"Passiven",
		"Gewinn- und Verlustrechnung 2026",
		"Reingewinn",
		"1.000,00", // Total Aktiven / Total Passiven
		"200,00",   // negated Verbindlichkeiten
		"800,00",   // negated Eigenkapital
		"5.000,00", // Einnahmen
		"2.000,00", // Ausgaben
		"3.000,00", // Reingewinn
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

func TestRenderStatementsHTMLBalanceOnly(t *testing.T) {
	html, err := RenderStatementsHTML(StatementsDoc{
		Title:   "Test",
		Year:    "2026",
		Balance: sampleBalance(),
	})
	if err != nil {
		t.Fatalf("RenderStatementsHTML returned error: %v", err)
	}

	if strings.Contains(html, `class="pagebreak"`) {
		t.Errorf("did not expect a page break when no income statement is rendered")
	}
}

func TestFormatHuman(t *testing.T) {
	cases := map[float64]string{
		0:        "0,00",
		1000:     "1.000,00",
		-1234.5:  "-1.234,50",
		12345678: "12.345.678,00",
	}

	for input, want := range cases {
		if got := formatHuman(input); got != want {
			t.Errorf("formatHuman(%v) = %q, want %q", input, got, want)
		}
	}
}
