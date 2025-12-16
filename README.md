# beancount-tooling

## Docker

Colima (replacement for Docker)

```bash
colima start
```

Build Docker image

```bash
docker build --no-cache -t fava-local .
```

Start container

```bash
docker-compose up -d
open http://localhost:5001
```

Stop container

```bash
docker-compose down
```

bean-check (Überprüft die Kontobuchungen)

- Kein Output, wenn keine Fehler vorhanden sind.
- Wenn Fehler vorhanden sind, werden die aufgelistet.

```bash
docker-compose --profile tools run --rm beancheck
```

## Abfragen

Einkommen

```sql
SELECT
  date      AS Datum,
  narration AS Buchungstext,
  abs(number(units(position))) AS Betrag
FROM postings
WHERE account ~ '^Income:'
  AND abs(number(units(position))) > 0
ORDER BY date;
```

Ausgaben

```sql
SELECT
  date      AS Datum,
  narration AS Buchungstext,
  units(position) AS Betrag
FROM postings
WHERE account ~ '^Expenses:'
  AND abs(number(units(position))) > 0
ORDER BY date;
```

## Generate reports

```bash
mkdir -p exports

# Export income into csv
docker-compose run --rm \
  -e BEANCOUNT_FILE=/ledger/main.beancount \
  beancount sh -lc 'bean-query -f csv "$BEANCOUNT_FILE" "$(cat)"' \
  < queries/income-select.bql \
  > exports/income-select.csv

# Export expenses into csv
  docker-compose run --rm \
  -e BEANCOUNT_FILE=/ledger/main.beancount \
  beancount sh -lc 'bean-query -f csv "$BEANCOUNT_FILE" "$(cat)"' \
  < queries/expenses-select.bql \
  > exports/expenses-select.csv

# Convert to xlsx
docker-compose --profile tools run --rm csv2xlsx
 ```
