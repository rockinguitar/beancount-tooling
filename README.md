# Beancount Tooling

![Go](https://img.shields.io/badge/Go-CLI-00ADD8?logo=go)
![Docker](https://img.shields.io/badge/Docker-compose-2496ED?logo=docker)
![Beancount](https://img.shields.io/badge/Beancount-querying-5B3DF5)
![Fava](https://img.shields.io/badge/Fava-web%20UI-7A3E9D)
![Mise](https://img.shields.io/badge/mise-task%20runner-5E6AD2)

Small tooling repo for running a Beancount ledger in Fava, validating it with `bean-check`, and generating filtered XLSX reports through a Go CLI.

## Prerequisites

- Docker plus either Docker Desktop or Colima
- `mise`
- Go `1.26` for local CLI development and tests

If you use Colima:

```bash
colima start
```

## Repository layout

- `tooling/` contains the application module and CLI
- `queries/` contains parameterized BQL templates
- `example/` contains a tiny demo ledger
- `example/reports/` is the default demo output directory and stays out of git

## Configuration

The workflow is driven by environment variables. `mise.toml` provides demo defaults and you can override them in `mise.local.toml`.

```toml
[env]
FINANCE_DIR = "/path/to/your/ledger-directory"
REPORTS_DIR = "/path/to/your/reports-directory"
BEANCOUNT_FILENAME = "main.beancount"
FAVA_PORT = "5001"
BEANCOUNT_ENGINE = "docker"
```

`BEANCOUNT_FILENAME` is only the entry ledger filename or relative path inside `FINANCE_DIR`.

Examples:

- `BEANCOUNT_FILENAME = "main.beancount"`
- `BEANCOUNT_FILENAME = "personal/main.beancount"`

## Example workflow

Without any overrides, the repo uses `example/test.beancount`.

Build the image:

```bash
mise run build-image
```

Start Fava:

```bash
mise run up
```

Run `bean-check`:

```bash
mise run check
```

Generate an example expenses workbook:

```bash
FROM=2026-02 TO=2026-02 mise run report-expenses
```

Generate an example income workbook:

```bash
FROM=2026-01 TO=2026-12 mise run report-income
```

Run the raw CSV query:

```bash
FROM=2026-02 TO=2026-02 mise run query-expenses
```

Stop Fava:

```bash
mise run down
```

## CLI

The Go CLI renders query templates, executes `bean-query`, and writes styled `.xlsx` workbooks.

Query to stdout:

```bash
go run ./tooling/cmd/beantool query expenses --from 2026-02-01 --to 2026-02-28
```

Create a workbook:

```bash
go run ./tooling/cmd/beantool report expenses \
  --from 2026-02-01 \
  --to 2026-02-28 \
  --out ./example/reports/expenses-february.xlsx
```

By default the CLI uses `BEANCOUNT_ENGINE=docker`, so `bean-query` runs inside the `beancount-tools` service. If you already have Beancount installed locally, you can switch to:

```bash
BEANCOUNT_ENGINE=local go run ./tooling/cmd/beantool report income --from 2026-01 --to 2026-12
```

## Query templates

Each report is a named template under `queries/`.

Current examples:

- `queries/expenses.tmpl.bql`
- `queries/income.tmpl.bql`

They support optional `from` / `to` bounds and are rendered by the CLI before execution.

## Development

Run tests:

```bash
mise run test
```

You can override the demo configuration in `mise.local.toml`, which is already gitignored.

On this repository, the Docker-based workflow uses the standalone `docker-compose` command rather than `docker compose`.
