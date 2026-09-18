package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rockinguitar/beancount-tooling/tooling/internal/fava"
	"github.com/rockinguitar/beancount-tooling/tooling/internal/query"
	"github.com/rockinguitar/beancount-tooling/tooling/internal/report"
	"github.com/rockinguitar/beancount-tooling/tooling/internal/runner"
)

type settings struct {
	repoRoot          string
	financeDir        string
	reportsDir        string
	beancountFilename string
	engine            runner.Engine
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "query":
		return runQueryCommand(ctx, args[1:])
	case "report":
		return runReportCommand(ctx, args[1:])
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `beantool executes parameterized Beancount queries, creates XLSX reports,
and renders financial statements as HTML from a running Fava instance.

Usage:
  beantool query <name> [flags]
  beantool report <name> [flags]              template-based XLSX report (expenses, income, ...)
  beantool report balance [flags]             Bilanz via Fava API -> HTML
  beantool report income [flags]              Gewinn- und Verlustrechnung via Fava API -> HTML
  beantool report revisor [flags]             combined Bilanz + GuV in one HTML document

Examples:
  beantool query expenses --from 2026-01-01 --to 2026-03-31
  beantool report income --from 2026-01 --to 2026-12 --out ./example/reports/income-2026.xlsx
  beantool report revisor --year 2026 --out ./Ledger/reports/revisor-2026.html
  beantool report balance --as-of 2026-06-30 --fava-url http://localhost:5001

Environment:
  FINANCE_DIR          Host directory containing your ledger files
  REPORTS_DIR          Host directory where reports are written
  QUERIES_DIR          Directory with query templates (default <repo root>/queries)
  BEANCOUNT_FILENAME   Ledger entry filename or relative path inside FINANCE_DIR
  BEANCOUNT_ENGINE     Query execution engine: docker (default) or local
  FAVA_URL             Fava base URL (default http://localhost:$FAVA_PORT, port 5001)
  BEANCOUNT_SLUG       Fava bfile slug (default: auto-discovered from Fava redirect)
`)
}

func runQueryCommand(ctx context.Context, args []string) error {
	cfg := loadSettings()

	if len(args) == 0 {
		return errors.New("query requires a report name, e.g. `beantool query expenses`")
	}

	queryName := args[0]

	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var from string
	var to string
	var out string
	var engine string

	fs.StringVar(&from, "from", "", "Lower date bound (YYYY-MM or YYYY-MM-DD)")
	fs.StringVar(&to, "to", "", "Upper date bound (YYYY-MM or YYYY-MM-DD)")
	fs.StringVar(&out, "out", "", "Optional output CSV path")
	fs.StringVar(&engine, "engine", string(cfg.engine), "Execution engine: docker or local")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if fs.NArg() != 0 {
		return errors.New("query accepts flags after the report name, e.g. `beantool query expenses --from 2026-01-01`")
	}

	params, err := query.NormalizeParams(from, to)
	if err != nil {
		return err
	}

	renderedQuery, err := renderNamedQuery(queryName, params)
	if err != nil {
		return err
	}

	result, err := runner.RunQuery(ctx, runner.Request{
		Engine:            runner.Engine(engine),
		FinanceDir:        cfg.financeDir,
		BeancountFilename: cfg.beancountFilename,
		Query:             renderedQuery,
	})
	if err != nil {
		return err
	}

	if out == "" {
		_, err = os.Stdout.Write(result)
		return err
	}

	return writeFile(out, result)
}

func runReportCommand(ctx context.Context, args []string) error {
	cfg := loadSettings()

	if len(args) == 0 {
		return errors.New("report requires a report name, e.g. `beantool report expenses`")
	}

	reportName := args[0]

	switch reportName {
	case "balance", "income", "revisor":
		return runStatementCommand(ctx, reportName, args[1:], cfg)
	}

	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var from string
	var to string
	var out string
	var engine string

	fs.StringVar(&from, "from", "", "Lower date bound (YYYY-MM or YYYY-MM-DD)")
	fs.StringVar(&to, "to", "", "Upper date bound (YYYY-MM or YYYY-MM-DD)")
	fs.StringVar(&out, "out", "", "Output XLSX path")
	fs.StringVar(&engine, "engine", string(cfg.engine), "Execution engine: docker or local")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if fs.NArg() != 0 {
		return errors.New("report accepts flags after the report name, e.g. `beantool report expenses --from 2026-01-01`")
	}

	params, err := query.NormalizeParams(from, to)
	if err != nil {
		return err
	}

	renderedQuery, err := renderNamedQuery(reportName, params)
	if err != nil {
		return err
	}

	result, err := runner.RunQuery(ctx, runner.Request{
		Engine:            runner.Engine(engine),
		FinanceDir:        cfg.financeDir,
		BeancountFilename: cfg.beancountFilename,
		Query:             renderedQuery,
	})
	if err != nil {
		return err
	}

	rows, err := csv.NewReader(strings.NewReader(string(result))).ReadAll()
	if err != nil {
		return fmt.Errorf("parse bean-query CSV: %w", err)
	}

	if len(rows) == 0 {
		return errors.New("query returned no rows")
	}

	if out == "" {
		out = filepath.Join(cfg.reportsDir, reportName+".xlsx")
	} else {
		out = resolveRepoPath(cfg.repoRoot, out)
	}

	workbookTitle := toWorkbookTitle(reportName)
	if err := report.WriteWorkbook(out, workbookTitle, rows); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Wrote %s\n", out)
	return nil
}

func runStatementCommand(ctx context.Context, name string, args []string, cfg settings) error {
	fs := flag.NewFlagSet("report "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var year string
	var asOf string
	var timeFilter string
	var out string
	var favaURL string
	var slug string
	var format string

	fs.StringVar(&year, "year", "", "Fiscal year (YYYY), defaults to the current year")
	fs.StringVar(&asOf, "as-of", "", "Exact balance-sheet date (YYYY-MM-DD)")
	fs.StringVar(&timeFilter, "time", "", "Raw Fava time filter, overrides --year and --as-of")
	fs.StringVar(&out, "out", "", "Output HTML path")
	fs.StringVar(&favaURL, "fava-url", "", "Fava base URL (default FAVA_URL or http://localhost:5001)")
	fs.StringVar(&slug, "slug", "", "Fava bfile slug (default BEANCOUNT_SLUG or auto-discovery)")
	fs.StringVar(&format, "format", "html", "Output format (only html is supported)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 0 {
		return fmt.Errorf("%s accepts flags only, e.g. `beantool report %s --year 2026`", name, name)
	}

	if format != "html" {
		return fmt.Errorf("unsupported format %q (only html is supported)", format)
	}

	filter, yearLabel, asOfLabel, err := resolvePeriod(year, asOf, timeFilter)
	if err != nil {
		return err
	}

	client := fava.NewClient(resolveFavaURL(favaURL))
	if slug == "" {
		slug = envOrDefault("BEANCOUNT_SLUG", "")
	}
	if slug != "" {
		client.SetSlug(slug)
	}

	doc := report.StatementsDoc{
		Title: fallbackLedgerTitle(ctx, client),
		Year:  yearLabel,
		AsOf:  asOfLabel,
	}

	switch name {
	case "balance", "revisor":
		balance, err := client.TreeReport(ctx, fava.KindBalanceSheet, filter)
		if err != nil {
			return err
		}
		doc.Balance = balance
	}

	if name == "income" || name == "revisor" {
		income, err := client.TreeReport(ctx, fava.KindIncomeStatement, filter)
		if err != nil {
			return err
		}
		doc.Income = income
	}

	html, err := report.RenderStatementsHTML(doc)
	if err != nil {
		return fmt.Errorf("render %s report: %w", name, err)
	}

	if out == "" {
		out = filepath.Join(cfg.reportsDir, fmt.Sprintf("%s-%s.html", name, yearLabel))
	} else {
		out = resolveRepoPath(cfg.repoRoot, out)
	}

	if err := writeFile(out, []byte(html)); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Wrote %s\n", out)
	return nil
}

func resolvePeriod(year, asOf, raw string) (filter string, yearLabel string, asOfLabel string, err error) {
	now := strconv.Itoa(time.Now().Year())

	if raw != "" {
		label := now
		if len(raw) >= 4 && isDigits(raw[:4]) {
			label = raw[:4]
		}
		return raw, label, asOfLabelFor(raw), nil
	}

	if asOf != "" {
		parsed, parseErr := time.Parse("2006-01-02", asOf)
		if parseErr != nil {
			return "", "", "", fmt.Errorf("as-of date %q must use YYYY-MM-DD", asOf)
		}
		return asOf, strconv.Itoa(parsed.Year()), "per " + parsed.Format("02.01.2006"), nil
	}

	if year == "" {
		year = now
	}
	if !isDigits(year) || len(year) != 4 {
		return "", "", "", fmt.Errorf("year %q must be YYYY", year)
	}

	return year, year, "per 31.12." + year, nil
}

func asOfLabelFor(raw string) string {
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return "per " + parsed.Format("02.01.2006")
	}

	if len(raw) == 7 && raw[4] == '-' {
		if parsed, err := time.Parse("2006-01", raw); err == nil {
			return "per " + parsed.AddDate(0, 1, -1).Format("02.01.2006")
		}
	}

	if len(raw) >= 4 && isDigits(raw[:4]) {
		return "per 31.12." + raw[:4]
	}

	return ""
}

func isDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func resolveFavaURL(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if base := envOrDefault("FAVA_URL", ""); base != "" {
		return base
	}
	port := envOrDefault("FAVA_PORT", "5001")
	return "http://localhost:" + port
}

func fallbackLedgerTitle(ctx context.Context, client *fava.Client) string {
	data, err := client.LedgerData(ctx)
	if err != nil {
		return "Finanzbericht"
	}
	if data.Options.Title == "" {
		return "Finanzbericht"
	}
	return data.Options.Title
}

func renderNamedQuery(name string, params query.Params) (string, error) {
	dir, err := queriesDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, name+".tmpl.bql")
	return query.RenderTemplateFile(path, params)
}

func queriesDir() (string, error) {
	if custom := envOrDefault("QUERIES_DIR", ""); custom != "" {
		return resolveRepoPath(".", custom), nil
	}

	repoRoot, err := detectRepoRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, "queries"), nil
}

func loadSettings() settings {
	repoRoot, err := detectRepoRoot()
	if err != nil {
		repoRoot = "."
	}

	financeRaw := envOrDefault("FINANCE_DIR", filepath.Join(repoRoot, "example"))
	reportsRaw := envOrDefault("REPORTS_DIR", filepath.Join(repoRoot, "example", "reports"))
	return settings{
		repoRoot:          repoRoot,
		financeDir:        resolveRepoPath(repoRoot, financeRaw),
		reportsDir:        resolveRepoPath(repoRoot, reportsRaw),
		beancountFilename: envOrDefault("BEANCOUNT_FILENAME", "test.beancount"),
		engine:            runner.Engine(envOrDefault("BEANCOUNT_ENGINE", string(runner.EngineDocker))),
	}
}

func detectRepoRoot() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	candidates := []string{
		workingDir,
		filepath.Dir(workingDir),
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		if _, err := os.Stat(filepath.Join(candidate, "queries")); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("could not locate repository root containing queries/")
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func resolveRepoPath(repoRoot, value string) string {
	value = expandHome(value)

	if value == "" || filepath.IsAbs(value) {
		return value
	}

	return filepath.Join(repoRoot, value)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	return nil
}

func toWorkbookTitle(name string) string {
	title := strings.ReplaceAll(name, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.TrimSpace(title)
	title = titleCase(title)
	if title == "" {
		return "Report"
	}

	if len(title) > 31 {
		return title[:31]
	}

	return title
}

func titleCase(value string) string {
	parts := strings.Fields(value)
	for index, part := range parts {
		if part == "" {
			continue
		}

		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}

	return strings.Join(parts, " ")
}
