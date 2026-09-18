# Beancount Tooling

![Go](https://img.shields.io/badge/Go-CLI-00ADD8?logo=go)
![Docker](https://img.shields.io/badge/Docker-compose-2496ED?logo=docker)
![Beancount](https://img.shields.io/badge/Beancount-querying-5B3DF5)
![Fava](https://img.shields.io/badge/Fava-web%20UI-7A3E9D)
![Mise](https://img.shields.io/badge/mise-task%20runner-5E6AD2)

Small tooling repo for running a Beancount ledger in Fava, validating it with `bean-check`, and generating filtered XLSX reports through a Go CLI.

The CLI also talks to the Fava JSON API and renders a **self-contained, print-ready HTML document** containing the Bilanz (balance sheet) and Gewinn- und Verlustrechnung (income statement) formatted for a Revisor (auditor).

## Prerequisites

- Docker plus either Docker Desktop or Colima
- `mise`
- Go `1.26` for local CLI development and tests

If you use Colima:

```bash
colima start
```

## Repository layout

- `tooling/` — Go CLI and internal packages (Fava client, report renderer)
- `queries/` — parameterized BQL templates (example ledger only; put your own templates in your ledger directory)
- `example/` — tiny demo ledger and demo output

## Per-ledger configuration

This repo ships sensible defaults for the `example/` demo ledger.
To point at a real ledger, add a `mise.toml` **inside the ledger directory** setting absolute paths:

```toml
[env]
TOOLING_DIR = "/path/to/beancount-tooling"
FINANCE_DIR = "/path/to/your-ledger"
REPORTS_DIR = "/path/to/your-ledger/reports"
QUERIES_DIR = "/path/to/your-ledger/queries"
BEANCOUNT_FILENAME = "main.beancount"
BEANCOUNT_ENGINE = "docker"
FAVA_PORT = "5001"
FAVA_URL = "http://localhost:5001"
```

All tasks below can then be run from the ledger directory (e.g. `mise run revisor`).

## Available statements (Fava API → HTML)

These commands talk to a running Fava instance and render an HTML file.

### Bilanz (balance sheet)

```bash
YEAR=2026 mise run statement-balance
# or from the CLI:
go run ./tooling/cmd/beantool report balance --year 2026 --out ./report-balance.html
```

### Gewinn- und Verlustrechnung (income statement)

```bash
YEAR=2026 mise run statement-income
```

### Revisor (Bilanz + GuV in one document)

```bash
YEAR=2026 mise run statement-revisor
```

## XLSX reports (bean-query templates)

```bash
FROM=2026-01 TO=2026-12 mise run report-expenses
FROM=2026-01 TO=2026-12 mise run report-income
FROM=2026-01-01 TO=2026-12-31 mise run reports-all
```

Add a named `.tmpl.bql` file to your ledger's `queries/` directory and use `query <name>` or `report <name>` to run it.

## Development

```bash
mise run build-image
mise start
mise run check
mise run test
mise stop
```

Run tests directly:

```bash
cd tooling && go test ./...
```
