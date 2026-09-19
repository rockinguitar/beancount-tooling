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
		"Einnahmen",
		"Ausgaben",
		"Reingewinn",
		"1.000,00", // Total Aktiven / Total Passiven
		"200,00",   // negated Verbindlichkeiten
		"800,00",   // negated Eigenkapital
		"5.000,00", // Einnahmen
		"2.000,00", // Ausgaben
		"3.000,00", // Reingewinn
		`class="section balance"`,
		`class="section income"`,
		"print-color-adjust",
		`title="Vermoegen:Bank:Giro"`,
		">Giro</td>",
		">Fest</td>",
		">Vorauszahlungen</td>",
		">Anfangsbestand per 31.12.2023</td>",
		">Mitgliedsbeitraege</td>",
		">Betrieb</td>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}

	for _, unwanted := range []string{
		">Vermoegen:Bank:Giro</td>",
		">Vermoegen:Bank:Fest</td>",
		">Verbindlichkeiten:Vorauszahlungen</td>",
		">Eigenkapital:Anfangsbestand</td>",
		">Einnahmen:Mitgliedsbeitraege</td>",
		">Ausgaben:Betrieb</td>",
		"Total Vermögen",
		"Total Verbindlichkeiten",
		"Total Eigenkapital",
	} {
		if strings.Contains(html, unwanted) {
			t.Errorf("rendered HTML still contains %q", unwanted)
		}
	}
}

func TestRenderStatementsHTMLSideBySide(t *testing.T) {
	html, err := RenderStatementsHTML(StatementsDoc{
		Title:   "Test",
		Year:    "2026",
		Balance: sampleBalance(),
		Income:  sampleIncome(),
	})
	if err != nil {
		t.Fatalf("RenderStatementsHTML returned error: %v", err)
	}

	rows := strings.Split(html, "<tr>")

	foundBalancePair := false
	foundIncomePair := false
	foundBalanceTotals := false
	foundIncomeTotals := false
	for _, fragment := range rows {
		if strings.Contains(fragment, ">Giro</td>") && strings.Contains(fragment, ">Vorauszahlungen</td>") {
			foundBalancePair = true
		}
		if strings.Contains(fragment, ">Mitgliedsbeitraege</td>") && strings.Contains(fragment, ">Betrieb</td>") {
			foundIncomePair = true
		}
		if strings.Contains(fragment, ">Total Aktiven</td>") && strings.Contains(fragment, ">Total Passiven</td>") {
			foundBalanceTotals = true
		}
		if strings.Contains(fragment, ">Total Einnahmen</td>") && strings.Contains(fragment, ">Total Ausgaben</td>") {
			foundIncomeTotals = true
		}
	}

	for name, ok := range map[string]bool{
		"Aktiven/Passiven side by side":   foundBalancePair,
		"Einnahmen/Ausgaben side by side": foundIncomePair,
		"Bilanz totals on same line":      foundBalanceTotals,
		"GuV totals on same line":         foundIncomeTotals,
	} {
		if !ok {
			t.Errorf("expected %s in a single table row", name)
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

	if strings.Contains(html, `class="section income"`) {
		t.Errorf("did not expect an income section in a balance-only report")
	}
	if strings.Contains(html, "Gewinn- und Verlustrechnung") {
		t.Errorf("did not expect income statement content in a balance-only report, got %q", html)
	}
	if !strings.Contains(html, `class="section balance"`) {
		t.Errorf("expected a balance section in a balance-only report")
	}
}

func TestDisplayAccount(t *testing.T) {
	cases := map[string]string{
		"Eigenkapital:Earnings":          "Jahresergebnis",
		"Eigenkapital:Earnings:Current":  "Laufendes Jahr",
		"Eigenkapital:Earnings:Previous": "Previous",
		"Eigenkapital:Conversions":       "Umrechnungen",
		"Eigenkapital:Anfangsbestand":    "Anfangsbestand per 31.12.2023",
		"Vermoegen:Bank:Giro":            "Giro",
		"Ausgaben:Gebuehren:Current":     "Current",
	}

	for input, want := range cases {
		if got := displayAccount(input); got != want {
			t.Errorf("displayAccount(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRenderStatementsHTMLReservedAndZeroGroups(t *testing.T) {
	html, err := RenderStatementsHTML(StatementsDoc{
		Title: "Test",
		Year:  "2026",
		Balance: &fava.TreeReport{
			Trees: []fava.TreeNode{
				{
					Account:         "Vermoegen",
					BalanceChildren: fava.Amount{"NOK": "500.00"},
					Children: []fava.TreeNode{
						{Account: "Vermoegen:Bank:Giro", Balance: fava.Amount{"NOK": "500.00"}, BalanceChildren: fava.Amount{"NOK": "500.00"}},
					},
				},
				{Account: "Verbindlichkeiten", BalanceChildren: fava.Amount{}},
				{
					Account:         "Eigenkapital",
					BalanceChildren: fava.Amount{"NOK": "-500.00"},
					Children: []fava.TreeNode{
						{Account: "Eigenkapital:Earnings:Current", Balance: fava.Amount{"NOK": "-80.00"}, BalanceChildren: fava.Amount{"NOK": "-80.00"}},
						{Account: "Eigenkapital:Earnings:Previous", Balance: fava.Amount{"NOK": "-420.00"}, BalanceChildren: fava.Amount{"NOK": "-420.00"}},
						{Account: "Eigenkapital:Conversions", BalanceChildren: fava.Amount{}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderStatementsHTML returned error: %v", err)
	}

	for _, want := range []string{
		"Laufendes Jahr",
		"Previous",
		">Previous</td>",
		"Total Aktiven",
		"Total Passiven",
		`title="Eigenkapital:Earnings:Current"`,
		`title="Eigenkapital:Earnings:Previous"`,
		"420,00", // negated previous-year result
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}

	for _, unwanted := range []string{
		">Verbindlichkeiten<",
		"Conversions",
		"Umrechnungen",
		"Total Verbindlichkeiten",
	} {
		if strings.Contains(html, unwanted) {
			t.Errorf("rendered HTML should not contain %q", unwanted)
		}
	}

	if !strings.Contains(html, ">500,00</span>") && !strings.Contains(html, "500,00&nbsp;NOK") {
		t.Errorf("expected the total passiven of 500,00 to be rendered")
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
