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
	"strings"

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
	fmt.Fprintf(os.Stderr, `beantool executes parameterized Beancount queries and creates XLSX reports.

Usage:
  beantool query <name> [flags]
  beantool report <name> [flags]

Examples:
  beantool query expenses --from 2026-01-01 --to 2026-03-31
  beantool report income --from 2026-01 --to 2026-12 --out ./example/reports/income-2026.xlsx

Environment:
  FINANCE_DIR          Host directory containing your ledger files
  REPORTS_DIR          Host directory where reports are written
  BEANCOUNT_FILENAME   Ledger entry filename or relative path inside FINANCE_DIR
  BEANCOUNT_ENGINE     Query execution engine: docker (default) or local
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

func renderNamedQuery(name string, params query.Params) (string, error) {
	repoRoot, err := detectRepoRoot()
	if err != nil {
		return "", err
	}

	path := filepath.Join(repoRoot, "queries", name+".tmpl.bql")
	return query.RenderTemplateFile(path, params)
}

func loadSettings() settings {
	repoRoot, err := detectRepoRoot()
	if err != nil {
		repoRoot = "."
	}

	return settings{
		repoRoot:          repoRoot,
		financeDir:        resolveRepoPath(repoRoot, envOrDefault("FINANCE_DIR", filepath.Join(repoRoot, "example"))),
		reportsDir:        resolveRepoPath(repoRoot, envOrDefault("REPORTS_DIR", filepath.Join(repoRoot, "example", "reports"))),
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
	if value == "" || filepath.IsAbs(value) {
		return value
	}

	return filepath.Join(repoRoot, value)
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
