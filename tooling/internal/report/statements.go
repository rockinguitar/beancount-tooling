package report

import (
	"bytes"
	"html/template"
	"sort"
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

// RenderStatementsHTML renders a self-contained, print-ready HTML document
// containing the balance sheet and the income statement.
func RenderStatementsHTML(doc StatementsDoc) (string, error) {
	balance := renderBalance(doc)
	income := renderIncome(doc)

	var rows []row
	rows = append(rows, balance...)
	if len(balance) > 0 && len(income) > 0 {
		rows = append(rows, row{Kind: rowPageBreak})
	}
	rows = append(rows, income...)

	tmpl, err := template.New("statements").Parse(statementsTemplate)
	if err != nil {
		return "", err
	}

	data := struct {
		Title string
		Year  string
		Rows  []row
	}{
		Title: doc.Title,
		Year:  doc.Year,
		Rows:  rows,
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}

	return out.String(), nil
}

type rowKind int

const (
	rowCaption rowKind = iota
	rowGroup
	rowLeaf
	rowTotal
	rowPageBreak
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
	Accent   string
	Negative bool
	Money    []money
}

func (r row) IsPageBreak() bool { return r.Kind == rowPageBreak }

func (r row) IsCaption() bool { return r.Kind == rowCaption }

func (r row) Class() string {
	switch r.Kind {
	case rowGroup:
		return "group"
	case rowLeaf:
		return "leaf"
	default:
		return "total"
	}
}

func renderBalance(doc StatementsDoc) []row {
	if doc.Balance == nil || len(doc.Balance.Trees) == 0 {
		return nil
	}

	trees := doc.Balance.Trees

	asOfLabel := doc.AsOf
	if asOfLabel == "" {
		asOfLabel = balanceAsOf(doc.Year)
	}

	var rows []row
	rows = append(rows, row{Kind: rowCaption, Label: "Bilanz " + asOfLabel, Accent: "cap-balance"})

	rows = append(rows, row{Kind: rowCaption, Label: "Aktiven", Accent: "cap-balance"})
	rows = append(rows, renderTree(trees[0], false)...)
	rows = append(rows, totalRow("Total Aktiven", trees[0].BalanceChildren, false))

	if len(trees) > 1 {
		rows = append(rows, row{Kind: rowCaption, Label: "Passiven", Accent: "cap-balance"})
		for _, tree := range trees[1:] {
			rows = append(rows, renderTree(tree, true)...)
		}
		rows = append(rows, totalRow("Total Passiven", sumTree(trees[1:], true), false))
	}

	return rows
}

func renderIncome(doc StatementsDoc) []row {
	if doc.Income == nil || len(doc.Income.Trees) == 0 {
		return nil
	}

	trees := doc.Income.Trees

	var rows []row
	rows = append(rows, row{Kind: rowCaption, Label: "Gewinn- und Verlustrechnung " + doc.Year, Accent: "cap-income"})

	income := trees[0]
	rows = append(rows, renderTree(income, true)...)

	var expenses fava.TreeNode
	if len(trees) > 2 {
		expenses = trees[2]
		rows = append(rows, renderTree(expenses, false)...)

		profit := negate(addMaps(income.BalanceChildren, expenses.BalanceChildren))
		label := "Reingewinn"
		negative := false
		if sumValues(profit) < 0 {
			label = "Reinverlust"
			negative = true
		}
		rows = append(rows, row{Kind: rowTotal, Level: 0, Label: label, Negative: negative, Money: amounts(profit)})
	}

	return rows
}

func renderTree(tree fava.TreeNode, negateBalance bool) []row {
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

	result = append(result, totalRow("Total "+tree.Account, tree.BalanceChildren, negateBalance))
	return result
}

// shortAccount returns the last segment of a hierarchical account name.
// "Vermoegen:Bank:Anlage" becomes "Anlage" — the nesting is conveyed by
// indentation, and the full path is available via the row's Full field.
func shortAccount(account string) string {
	if index := strings.LastIndex(account, ":"); index >= 0 {
		return account[index+1:]
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
	"Previous":    "Vorjahr",
}

// displayAccount is the visible label for an account: its last segment,
// translated when the account is one of Beancount's reserved equity accounts.
func displayAccount(account string) string {
	if isReservedEquityAccount(account) {
		if label, ok := reservedEquityLabels[shortAccount(account)]; ok {
			return label
		}
	}
	return shortAccount(account)
}

func isReservedEquityAccount(account string) bool {
	return strings.HasSuffix(account, ":Earnings") ||
		strings.HasSuffix(account, ":Earnings:Current") ||
		strings.HasSuffix(account, ":Earnings:Previous") ||
		strings.HasSuffix(account, ":Conversions")
}

func totalRow(label string, values fava.Amount, negateValues bool) row {
	return row{Kind: rowTotal, Level: 0, Label: label, Money: amounts(negateOpt(values, negateValues))}
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
	currencies := make([]string, 0, len(values))
	for currency := range values {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)

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
	parts := strings.SplitN(raw, ".", 2)
	intPart, fracPart := parts[0], parts[1]

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
    @page { size: A4; margin: 18mm 16mm; }
    * { box-sizing: border-box; }
    body { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 11pt; color: #1f2937; margin: 0; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    .header { border-bottom: 3px solid #3b5b8c; padding-bottom: 6pt; margin-bottom: 12pt; }
    h1 { font-size: 18pt; margin: 0 0 2pt 0; color: #17202a; }
    .subtitle { color: #6b7280; margin: 0; }
    table { width: 100%; border-collapse: collapse; }
    tr.pagebreak td { height: 0; padding: 0; border: 0; }
    td { padding: 1.5pt 4pt; vertical-align: top; }
    td.num { text-align: right; white-space: nowrap; }
    .caption td { font-weight: 700; font-size: 12pt; padding: 5pt 8pt; letter-spacing: .02em; border-radius: 3pt; }
    .caption.cap-balance td { color: #1e3a5f; background: #eef2f8; border-top: 1px solid #c6d3e4; border-bottom: 1px solid #c6d3e4; border-left: 4px solid #3b5b8c; }
    .caption.cap-income td { color: #1f4d43; background: #eaf3f0; border-top: 1px solid #c3ded6; border-bottom: 1px solid #c3ded6; border-left: 4px solid #2d7a6e; }
    .group td, .total td { font-weight: 700; }
    .leaf td { color: #374151; }
    tr.group td { border-bottom: 1px dotted #bbb; padding-top: 4pt; color: #28354a; }
    tr.total td { border-top: 2px solid #111; padding-top: 4pt; background: #f3f4f6; }
    tr.total.negative td { color: #b3261e; border-top-color: #b3261e; }
    .dim { color: #8a919c; font-weight: 400; font-size: 9pt; }
    .signatures { margin-top: 28pt; width: 100%; }
    .signatures td { width: 50%; padding-top: 22pt; border-top: 1px solid #111; }
    .caption, .total { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    @media print {
      tr, table { page-break-inside: auto; }
      .caption { page-break-after: avoid; }
    }
  </style>
</head>
<body>
  <div class="header">
    <h1>{{.Title}}</h1>
    <div class="subtitle">Bilanz und Gewinn- und Verlustrechnung – Geschaeftsjahr {{.Year}}</div>
  </div>
  <table>
    {{range .Rows}}{{if .IsPageBreak}}<tr class="pagebreak"><td>&nbsp;</td><td>&nbsp;</td></tr>{{else if .IsCaption}}<tr class="caption {{.Accent}}"><td colspan="2">{{.Label}}</td></tr>{{else}}<tr class="{{.Class}}{{if .Negative}} negative{{end}}"><td style="padding-left: {{.Level}}.5em"{{if .Full}} title="{{.Full}}"{{end}}>{{.Label}}</td><td class="num">{{range $i, $m := .Money}}{{if $i}}<br/>{{end}}{{$m.Amount}}&nbsp;{{$m.Currency}}{{end}}</td></tr>{{end}}{{end}}
  </table>
  <table class="signatures">
    <tr>
      <td>Ort, Datum</td>
      <td>Unterschrift</td>
    </tr>
  </table>
  <p class="dim">Erstellt mit beantool aus den Beancount-Daten.</p>
</body>
</html>`
