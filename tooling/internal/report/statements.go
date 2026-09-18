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
	Kind  rowKind
	Level int
	Label string
	Money []money
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
	rows = append(rows, row{Kind: rowCaption, Label: "Bilanz " + asOfLabel})

	rows = append(rows, row{Kind: rowCaption, Label: "Aktiven"})
	rows = append(rows, renderTree(trees[0], false)...)
	rows = append(rows, totalRow("Total Aktiven", trees[0].BalanceChildren, false))

	if len(trees) > 1 {
		rows = append(rows, row{Kind: rowCaption, Label: "Passiven"})
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
	rows = append(rows, row{Kind: rowCaption, Label: "Gewinn- und Verlustrechnung " + doc.Year})

	income := trees[0]
	rows = append(rows, renderTree(income, true)...)

	var expenses fava.TreeNode
	if len(trees) > 2 {
		expenses = trees[2]
		rows = append(rows, renderTree(expenses, false)...)

		profit := negate(addMaps(income.BalanceChildren, expenses.BalanceChildren))
		label := "Reingewinn"
		if sumValues(profit) < 0 {
			label = "Reinverlust"
		}
		rows = append(rows, row{Kind: rowTotal, Level: 0, Label: label, Money: amounts(profit)})
	}

	return rows
}

func renderTree(tree fava.TreeNode, negateBalance bool) []row {
	result := []row{
		{Kind: rowGroup, Level: 0, Label: tree.Account, Money: amounts(negateOpt(tree.BalanceChildren, negateBalance))},
	}

	var collect func(children []fava.TreeNode, level int)
	collect = func(children []fava.TreeNode, level int) {
		for _, child := range children {
			if len(child.Children) > 0 {
				result = append(result, row{Kind: rowGroup, Level: level, Label: child.Account, Money: amounts(negateOpt(child.BalanceChildren, negateBalance))})
				collect(child.Children, level+1)
				continue
			}

			balance := negateOpt(child.Balance, negateBalance)
			if isZero(balance) {
				continue
			}
			result = append(result, row{Kind: rowLeaf, Level: level, Label: child.Account, Money: amounts(balance)})
		}
	}
	collect(tree.Children, 1)

	result = append(result, totalRow("Total "+tree.Account, tree.BalanceChildren, negateBalance))
	return result
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
    body { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 11pt; color: #111; margin: 0; }
    h1 { font-size: 17pt; margin: 0 0 2pt 0; }
    .subtitle { color: #555; margin-bottom: 14pt; }
    table { width: 100%; border-collapse: collapse; }
    tr.pagebreak td { height: 0; padding: 0; border: 0; }
    .caption td { font-weight: 700; font-size: 12pt; border-top: 2px solid #111; border-bottom: 1px solid #111; padding: 6pt 0 4pt 0; letter-spacing: .02em; }
    td { padding: 1.5pt 4pt; vertical-align: top; }
    td.num { text-align: right; white-space: nowrap; }
    .group td, .total td { font-weight: 700; }
    tr.group td { border-bottom: 1px dotted #aaa; padding-top: 4pt; }
    tr.total td { border-top: 1px solid #111; padding-top: 4pt; }
    .dim { color: #999; font-weight: 400; font-size: 9pt; }
    .signatures { margin-top: 28pt; width: 100%; }
    .signatures td { width: 50%; padding-top: 22pt; border-top: 1px solid #111; }
    @media print {
      tr, table { page-break-inside: auto; }
      .caption { page-break-after: avoid; }
    }
  </style>
</head>
<body>
  <h1>{{.Title}}</h1>
  <div class="subtitle">Bilanz und Gewinn- und Verlustrechnung – Geschaeftsjahr {{.Year}}</div>
  <table>
    {{range .Rows}}{{if .IsPageBreak}}<tr class="pagebreak"><td>&nbsp;</td><td>&nbsp;</td></tr>{{else if .IsCaption}}<tr class="caption"><td colspan="2">{{.Label}}</td></tr>{{else}}<tr class="{{.Class}}"><td style="padding-left: {{.Level}}.5em">{{.Label}}</td><td class="num">{{range $i, $m := .Money}}{{if $i}}<br/>{{end}}{{$m.Amount}}&nbsp;{{$m.Currency}}{{end}}</td></tr>{{end}}{{end}}
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