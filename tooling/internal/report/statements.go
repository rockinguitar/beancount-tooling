package report

import (
	"bytes"
	"fmt"
	"html/template"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/rockinguitar/beancount-tooling/tooling/internal/fava"
)

// StatementsDoc is the data needed to render a Revisor report.
type StatementsDoc struct {
	Title   string
	Year    string
	AsOf    string
	Balance *fava.TreeReport
	Income  *fava.TreeReport
}

// section is one side-by-side ledger statement: the balance sheet or the
// income statement. Body holds the detail grid, Footer the aligned closing
// totals and Result optional full-width closing bars (e.g. Reingewinn).
type section struct {
	Class      string // css accent class, "balance" or "income"
	Title      string // e.g. "Bilanz per 31.12.2026"
	LeftTitle  string // column header, e.g. "Aktiven"
	RightTitle string // column header, e.g. "Passiven"
	Body       []gridRow
	Footer     []gridRow
	Result     []row
}

// gridRow aligns one left-column row with one right-column row so both sides
// sit on the same horizontal line. Either side may be nil when a column is
// shorter than the other.
type gridRow struct {
	Left  *row
	Right *row
}

type rowKind int

const (
	rowGroup rowKind = iota
	rowLeaf
	rowTotal
)

type money struct {
	Amount   string
	Currency string
}

type row struct {
	Kind     rowKind
	Level    int
	Label    string
	Full     string
	Negative bool
	Money    []money
}

func (r row) IsGroup() bool { return r.Kind == rowGroup }

func (r row) IsLeaf() bool { return r.Kind == rowLeaf }

// Indent returns the CSS padding-left for the row's nesting level.
func (r row) Indent() string {
	return fmt.Sprintf("%.1fem", .5+float64(r.Level))
}

// RenderStatementsHTML renders a self-contained, print-ready HTML document
// containing the balance sheet and the income statement.
func RenderStatementsHTML(doc StatementsDoc) (string, error) {
	var sections []section
	if balance := balanceSection(doc); balance != nil {
		sections = append(sections, *balance)
	}
	if income := incomeSection(doc); income != nil {
		sections = append(sections, *income)
	}

	subtitle := "Bilanz und Gewinn- und Verlustrechnung"
	if len(sections) == 1 {
		switch sections[0].Class {
		case "balance":
			subtitle = "Bilanz"
		case "income":
			subtitle = "Gewinn- und Verlustrechnung"
		}
	}

	tmpl, err := template.New("statements").Parse(statementsTemplate)
	if err != nil {
		return "", err
	}

	data := struct {
		Title    string
		Subtitle string
		Year     string
		Sections []section
	}{
		Title:    doc.Title,
		Subtitle: subtitle,
		Year:     doc.Year,
		Sections: sections,
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}

	return out.String(), nil
}

func balanceSection(doc StatementsDoc) *section {
	if doc.Balance == nil || len(doc.Balance.Trees) == 0 {
		return nil
	}

	trees := doc.Balance.Trees

	asOfLabel := doc.AsOf
	if asOfLabel == "" {
		asOfLabel = balanceAsOf(doc.Year)
	}

	var left, right []row
	left = treeRows(trees[0], false)
	for _, tree := range trees[1:] {
		right = append(right, treeRows(tree, true)...)
	}

	totalAssets := totalRow("Total Aktiven", trees[0].BalanceChildren, false)
	totalPassive := totalRow("Total Passiven", sumTree(trees[1:], true), false)

	return &section{
		Class:      "balance",
		Title:      "Bilanz " + asOfLabel,
		LeftTitle:  "Aktiven",
		RightTitle: "Passiven",
		Body:       alignColumns(left, right),
		Footer:     []gridRow{{Left: &totalAssets, Right: &totalPassive}},
	}
}

func incomeSection(doc StatementsDoc) *section {
	if doc.Income == nil || len(doc.Income.Trees) == 0 {
		return nil
	}

	trees := doc.Income.Trees
	income := trees[0]

	sec := &section{
		Class:      "income",
		Title:      "Gewinn- und Verlustrechnung " + doc.Year,
		LeftTitle:  "Einnahmen",
		RightTitle: "Ausgaben",
	}

	var expenses fava.TreeNode
	if len(trees) > 2 {
		expenses = trees[2]
		sec.Body = alignColumns(treeRows(income, true), treeRows(expenses, false))
	} else {
		sec.Body = alignColumns(treeRows(income, true), nil)
	}

	totalIncome := totalRow("Total Einnahmen", income.BalanceChildren, true)
	footer := gridRow{Left: &totalIncome}

	if len(trees) > 2 {
		totalExpenses := totalRow("Total Ausgaben", expenses.BalanceChildren, false)
		footer.Right = &totalExpenses

		profit := negate(addMaps(income.BalanceChildren, expenses.BalanceChildren))
		label := "Reingewinn"
		negative := false
		if sumValues(profit) < 0 {
			label = "Reinverlust"
			negative = true
		}
		sec.Result = []row{{Kind: rowTotal, Level: 0, Label: label, Negative: negative, Money: amounts(profit)}}
	}

	sec.Footer = []gridRow{footer}
	return sec
}

// treeRows renders one account subtree as hierarchy rows: the root group
// followed by its non-zero children. The closing total for the tree is not
// included; grand totals live in the section's footer instead.
func treeRows(tree fava.TreeNode, negateBalance bool) []row {
	rootBalance := negateOpt(tree.BalanceChildren, negateBalance)
	if isZero(rootBalance) {
		return nil
	}

	result := []row{
		{Kind: rowGroup, Level: 0, Label: tree.Account, Full: tree.Account, Money: amounts(rootBalance)},
	}

	var collect func(children []fava.TreeNode, level int)
	collect = func(children []fava.TreeNode, level int) {
		for _, child := range children {
			childBalance := negateOpt(child.BalanceChildren, negateBalance)
			if len(child.Children) > 0 {
				if isZero(childBalance) {
					continue
				}
				result = append(result, row{Kind: rowGroup, Level: level, Label: displayAccount(child.Account), Full: child.Account, Money: amounts(childBalance)})
				collect(child.Children, level+1)
				continue
			}

			balance := negateOpt(child.Balance, negateBalance)
			if isZero(balance) {
				continue
			}
			result = append(result, row{Kind: rowLeaf, Level: level, Label: displayAccount(child.Account), Full: child.Account, Money: amounts(balance)})
		}
	}
	collect(tree.Children, 1)

	return result
}

// alignColumns merges two row lists into a side-by-side grid, padding the
// shorter side so both columns share the same number of horizontals.
func alignColumns(left, right []row) []gridRow {
	max := len(left)
	if len(right) > max {
		max = len(right)
	}

	out := make([]gridRow, 0, max)
	for i := 0; i < max; i++ {
		var pair gridRow
		if i < len(left) {
			copy := left[i]
			pair.Left = &copy
		}
		if i < len(right) {
			copy := right[i]
			pair.Right = &copy
		}
		out = append(out, pair)
	}
	return out
}

// shortAccount returns the last segment of a hierarchical account name.
// "Vermoegen:Bank:Anlage" becomes "Anlage" — the nesting is conveyed by
// indentation, and the full path is available via the row's Full field.
func shortAccount(account string) string {
	if _, short, found := strings.CutLast(account, ":"); found {
		return short
	}
	return account
}

// reservedEquityLabels translates the fixed English segment names Beancount
// uses for its reserved equity accounts (Earnings/Conversions/Current/...).
// Only the root of those names is configurable (name_equity).
var reservedEquityLabels = map[string]string{
	"Earnings":    "Jahresergebnis",
	"Conversions": "Umrechnungen",
	"Current":     "Laufendes Jahr",
}

// anfangsbestandLabel is the display label for the opening-balances account,
// pinned to the fixed date on which the initial balances were booked.
const anfangsbestandLabel = "Anfangsbestand per 31.12.2023"

// displayAccount is the visible label for an account: its last segment,
// translated when the account is one of Beancount's reserved equity accounts.
func displayAccount(account string) string {
	short := shortAccount(account)
	if short == "Anfangsbestand" {
		return anfangsbestandLabel
	}
	if isReservedEquityAccount(account) {
		if label, ok := reservedEquityLabels[short]; ok {
			return label
		}
	}
	return short
}

func isReservedEquityAccount(account string) bool {
	return strings.HasSuffix(account, ":Earnings") ||
		strings.HasSuffix(account, ":Earnings:Current") ||
		strings.HasSuffix(account, ":Earnings:Previous") ||
		strings.HasSuffix(account, ":Conversions")
}

func totalRow(label string, values fava.Amount, negateValues bool) row {
	values = negateOpt(values, negateValues)
	return row{Kind: rowTotal, Level: 0, Label: label, Money: amounts(values), Negative: sumValues(values) < 0}
}

func negateOpt(values fava.Amount, doNegate bool) fava.Amount {
	if !doNegate {
		return values
	}
	return negate(values)
}

func negate(values fava.Amount) fava.Amount {
	out := make(fava.Amount, len(values))
	for currency, raw := range values {
		number, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			out[currency] = raw
			continue
		}
		out[currency] = fava.Decimal(formatRaw(-number))
	}
	return out
}

func sumTree(trees []fava.TreeNode, negateValues bool) fava.Amount {
	var lists []fava.Amount
	for _, tree := range trees {
		lists = append(lists, negateOpt(tree.BalanceChildren, negateValues))
	}
	return addMaps(lists...)
}

func addMaps(lists ...fava.Amount) fava.Amount {
	out := fava.Amount{}
	for _, entry := range lists {
		for currency, raw := range entry {
			number, err := strconv.ParseFloat(string(raw), 64)
			if err != nil {
				continue
			}
			current, _ := strconv.ParseFloat(string(out[currency]), 64)
			out[currency] = fava.Decimal(formatRaw(current + number))
		}
	}
	return out
}

func isZero(values fava.Amount) bool {
	for _, raw := range values {
		number, err := strconv.ParseFloat(string(raw), 64)
		if err != nil || number != 0 {
			return false
		}
	}
	return true
}

func sumValues(values fava.Amount) float64 {
	var sum float64
	for _, raw := range values {
		number, err := strconv.ParseFloat(string(raw), 64)
		if err == nil {
			sum += number
		}
	}
	return sum
}

func amounts(values fava.Amount) []money {
	currencies := slices.Collect(maps.Keys(values))
	slices.Sort(currencies)

	out := make([]money, 0, len(currencies))
	for _, currency := range currencies {
		number, err := strconv.ParseFloat(string(values[currency]), 64)
		if err != nil {
			continue
		}
		out = append(out, money{Amount: formatHuman(number), Currency: currency})
	}
	return out
}

func formatRaw(number float64) string {
	return strconv.FormatFloat(number, 'f', 2, 64)
}

func formatHuman(number float64) string {
	if number == 0 {
		number = 0
	}

	raw := strconv.FormatFloat(number, 'f', 2, 64)
	intPart, fracPart, _ := strings.Cut(raw, ".")

	negative := strings.HasPrefix(intPart, "-")
	if negative {
		intPart = strings.TrimPrefix(intPart, "-")
	}

	var grouped strings.Builder
	for index, digit := range intPart {
		if index > 0 && (len(intPart)-index)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}

	result := grouped.String() + "," + fracPart
	if negative {
		result = "-" + result
	}
	return result
}

func balanceAsOf(year string) string {
	if year == "" {
		return ""
	}
	return "per 31.12." + year
}

const statementsTemplate = `<!DOCTYPE html>
<html lang="de">
<head>
  <meta charset="utf-8"/>
  <title>{{.Title}} – Bilanz &amp; Erfolgsrechnung</title>
  <style>
    :root {
      --ink: #1f2a37;
      --muted: #64748b;
      --line: #e5e7eb;
      --soft: #f8fafc;
      --blue: #2563eb;
      --tint-blue: #eff4ff;
      --violet: #7c3aed;
      --tint-violet: #f5f3ff;
      --green: #059669;
      --tint-green: #ecfdf5;
      --orange: #d97706;
      --tint-orange: #fff7ed;
      --negative: #dc2626;
      --tint-negative: #fef2f2;
    }

    @page { size: A4 landscape; margin: 14mm 12mm 14mm; }

    * { box-sizing: border-box; }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif;
      font-size: 10.5pt;
      line-height: 1.5;
      color: var(--ink);
      margin: 32px;
      orphans: 2;
      widows: 2;
      -webkit-print-color-adjust: exact;
      print-color-adjust: exact;
    }

    /* document header */
    .doc-header {
      display: flex;
      align-items: flex-end;
      justify-content: space-between;
      gap: 16pt;
      padding-bottom: 12pt;
      border-bottom: 2.5px solid var(--ink);
      margin-bottom: 20pt;
    }
    .doc-header .title { display: flex; align-items: center; gap: 10pt; }
    .doc-header h1 { font-size: 19pt; letter-spacing: -.01em; margin: 0; line-height: 1.15; }
    .doc-header .sub { margin-top: 3pt; color: var(--muted); font-size: 9.5pt; }
    .doc-header .year {
      padding: 5pt 12pt;
      border: 1px solid var(--line);
      border-radius: 999pt;
      background: var(--soft);
      color: var(--muted);
      font-size: 9.5pt;
      font-weight: 600;
      letter-spacing: .06em;
      white-space: nowrap;
    }

    /* sections */
    .section { margin-bottom: 22pt; }
    .section + .section { page-break-before: always; }

    .section-head {
      display: flex;
      align-items: center;
      gap: 8pt;
      margin: 0 0 10pt;
      page-break-after: avoid;
    }
    .section-head h2 { font-size: 13.5pt; letter-spacing: -.01em; margin: 0; }

    .card {
      border: 1px solid var(--line);
      border-radius: 11pt;
      overflow: hidden;
      background: #fff;
      box-shadow: 0 1px 2px rgba(31, 42, 55, .05);
    }

    /* ledger table */
    table.ledger { width: 100%; border-collapse: collapse; }
    table.ledger .c-label { width: 33%; }
    table.ledger .c-amount { width: 17%; }

    table.ledger thead th {
      text-align: left;
      text-transform: uppercase;
      letter-spacing: .09em;
      font-weight: 700;
      font-size: 8pt;
      padding: 9pt 12pt;
      border-bottom: 1px solid var(--line);
      color: var(--ink);
    }
    table.ledger thead .th-amt { text-align: right; font-weight: 600; color: var(--muted); }
    table.ledger thead .th-side-l { background: var(--tint-blue); }
    table.ledger thead .th-side-r { background: var(--tint-violet); }
    .section.income table.ledger thead .th-side-l { background: var(--tint-green); }
    .section.income table.ledger thead .th-side-r { background: var(--tint-orange); }
    table.ledger thead .tick {
      display: inline-block;
      width: 7pt;
      height: 7pt;
      border-radius: 2pt;
      margin-right: 6pt;
      vertical-align: 1pt;
    }
    table.ledger thead .th-side-l .tick { background: var(--blue); }
    table.ledger thead .th-side-r .tick { background: var(--violet); }
    .section.income table.ledger thead .th-side-l .tick { background: var(--green); }
    .section.income table.ledger thead .th-side-r .tick { background: var(--orange); }

    table.ledger tbody td {
      padding: 6pt 12pt;
      border-bottom: 1px solid var(--line);
      vertical-align: top;
    }
    table.ledger tbody tr:nth-child(even) td { background: #fafbfc; }
    table.ledger tbody tr:last-child td { border-bottom: none; }

    table.ledger td.lbl.group { font-weight: 700; }
    table.ledger td.lbl.leaf { color: #374151; }
    table.ledger td.lbl.negative, table.ledger td.amt.negative { color: var(--negative); }
    table.ledger td.amt {
      text-align: right;
      white-space: nowrap;
      font-variant-numeric: tabular-nums;
    }

    table.ledger tfoot td {
      padding: 8pt 12pt;
      border-top: 2px solid var(--ink);
      background: var(--soft);
      font-weight: 700;
    }

    /* closing bar, e.g. Reingewinn / Reinverlust */
    .result {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12pt;
      margin: 12pt 14pt 14pt;
      padding: 11pt 14pt;
      border: 1px solid var(--line);
      border-left: 4pt solid var(--green);
      border-radius: 8pt;
      font-weight: 800;
      font-size: 12pt;
      page-break-before: avoid;
    }
    .result.positive { color: #065f46; background: var(--tint-green); }
    .result.negative { color: var(--negative); background: var(--tint-negative); border-left-color: var(--negative); }
    .result .amt { font-variant-numeric: tabular-nums; white-space: nowrap; }

    /* signatures and footer note */
    .signatures {
      display: flex;
      gap: 32pt;
      margin-top: 34pt;
    }
    .sig {
      flex: 1;
      padding-top: 8pt;
      border-top: 1px solid var(--ink);
      color: var(--muted);
      font-size: 9.5pt;
    }

    @media print {
      body { background: #fff; margin: 0; }
      .section + .section { page-break-before: always; }
      table.ledger thead { display: table-header-group; }
      table.ledger tbody tr, table.ledger tfoot tr { page-break-inside: avoid; }
    }
  </style>
</head>
<body>
  {{define "side"}}
    {{if .}}
    <td class="lbl{{if .IsGroup}} group{{end}}{{if .IsLeaf}} leaf{{end}}{{if .Negative}} negative{{end}}"{{if .Full}} title="{{.Full}}"{{end}} style="padding-left: {{.Indent}}">{{.Label}}</td>
    <td class="amt{{if .Negative}} negative{{end}}">{{range $i, $m := .Money}}{{if $i}}<br/>{{end}}{{$m.Amount}}&nbsp;{{$m.Currency}}{{end}}</td>
    {{else}}
    <td class="lbl"></td>
    <td class="amt"></td>
    {{end}}
  {{end}}
  <header class="doc-header">
    <div>
      <div class="title"><h1>{{.Title}}</h1></div>
      <div class="sub">{{.Subtitle}}</div>
    </div>
    <div class="year">Geschäftsjahr {{.Year}}</div>
  </header>

  <main>
    {{range .Sections}}
    <section class="section {{.Class}}">
      <div class="section-head">
        <h2>{{.Title}}</h2>
      </div>
      <div class="card">
        <table class="ledger">
          <colgroup>
            <col class="c-label"/>
            <col class="c-amount"/>
            <col class="c-label"/>
            <col class="c-amount"/>
          </colgroup>
          <thead>
            <tr>
              <th class="th-side-l"><span class="tick"></span>{{.LeftTitle}}</th>
              <th class="th-amt">Betrag</th>
              <th class="th-side-r"><span class="tick"></span>{{.RightTitle}}</th>
              <th class="th-amt">Betrag</th>
            </tr>
          </thead>
          <tbody>
            {{range .Body}}
            <tr>
              {{template "side" .Left}}
              {{template "side" .Right}}
            </tr>
            {{end}}
          </tbody>
          {{if .Footer}}
          <tfoot>
            {{range .Footer}}
            <tr>
              {{template "side" .Left}}
              {{template "side" .Right}}
            </tr>
            {{end}}
          </tfoot>
          {{end}}
        </table>
        {{range .Result}}
        <div class="result {{if .Negative}}negative{{else}}positive{{end}}">
          <span class="label">{{.Label}}</span>
          <span class="amt">{{range $i, $m := .Money}}{{if $i}}<br/>{{end}}{{$m.Amount}}&nbsp;{{$m.Currency}}{{end}}</span>
        </div>
        {{end}}
      </div>
    </section>
    {{end}}
  </main>

  <footer>
    <div class="signatures">
      <div class="sig">Ort, Datum</div>
      <div class="sig">Unterschrift</div>
    </div>
    </footer>
</body>
</html>`
