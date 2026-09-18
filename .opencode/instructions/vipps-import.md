# Vipps → Beancount Import Process

Generate individual beancount member fee transactions from Vipps payout batches and DNB bank statements.

## Required Inputs

### 1. Vipps Export (TSV)
Export from Vipps admin → Reports → Transactions, filtered by merchant "CH-Klubb".

Columns (tab-separated):
| # | Field | Example |
|---|-------|---------|
| 1 | Merchant | CH-Klubb |
| 2 | Orgno | 102059 |
| 3 | Country | Norway |
| 4 | Status | Open amount |
| 5 | Timestamp | 2026-06-02 18:59:33 |
| 6 | Capture date | 2026-06-02 |
| 7 | Type | Capture / Fees retained / Payout scheduled |
| 8 | Amount | 200.00 |
| 9 | Running total | 200.00 |
| 10 | Fee | -3.50 |
| 11 | Net | 196.50 |
| 12 | Currency | NOK |
| 13 | Payer name | Carmen Müller |
| 14 | Phone | +47 xxxx 5261 |
| 15 | Text melding | Carmen Müller, Mitgliederbeitrag 2026 |
| 16 | Membranmerk | Einzelmitgliedschaft 200 NOK |
| 17+ | IDs | 36919152556, 13606122176 |

### 2. DNB Statement (TSV)
Export from DNB online banking for the relevant period.

Columns (tab-separated):
| # | Field | Example |
|---|-------|---------|
| 1 | Bokført dato | 06/04/2026 |
| 2 | Rentedato | 06/04/2026 |
| 3 | Transaksjonstype | Overføring innland |
| 4 | Forklarende tekst | Vipps Mobilepay As Utb. 2000369 Vippsnr 102059 |
| 5 | Inn | 196.50 |
| 6 | Ut | |
| 7 | Arkivref. | 798137395 |
| 8 | Referanse | 1331727 |
| 9 | Detaljer | |
| 10 | Status | B |

## Processing Steps

### Step 1: Parse Vipps — group into payout batches

Each **capture date** (col 6) forms one payout batch. For each batch you have:
- **Capture rows** (col 7 = `Capture`): one per member fee payment
- **Fees retained row** (col 7 = `Fees retained`): total batch fee
- **Payout scheduled row** (col 7 = `Payout scheduled`): contains `abrechnungs_id` in "Utb. {ID} Vippsnr {orgno}" (col 15/16 area)

### Step 2: Parse DNB — find Vipps transfers

Filter rows where:
- col 3 = `Overføring innland`
- col 4 contains `Vipps`

Extract `abrechnungs_id` from col 4 using regex: `Utb\. (\d{4,})`

Record:
- DNB booking date = col 1 → convert from `DD/MM/YYYY` to `YYYY-MM-DD`
- Net amount = col 5 (Inn)
- `abrechnungs_id` from regex match

### Step 3: Match batches to DNB entries

Match each Vipps payout batch to a DNB entry by `abrechnungs_id`.

### Step 4: Generate transactions

For each **Capture row** in a matched batch:

**Date** = DNB booking date (from Step 3 match)

**Narration** =
- Extract membership type from Membranmerk (col 16): starts with `Einzelmitgliedschaft` or `Familienmitgliedschaft`
- If Text melding (col 15) contains a different person's name than Payer name (col 13), append `für {Name}` to narration
  - e.g., payer "Rosmarie Chappel" with text "Für Alexandre Chappel" → `"Familienmitgliedschaft für Alexandre Chappel"`
  - e.g., payer "Marianne Maurer" with text "Beatrice Paech" → `"Einzelmitgliedschaft für Beatrice Paech"`
- If Text melding is empty or just repeats the payer name, use membership type only

**Metadata:**
```
  quelle: "Vipps"
  abrechnungs_id: "{batch ID}"
  zahler: "{Payer name from col 13}"
```

**Postings:**
```
  Assets:Bank:Giro                    {net} NOK     # col 11
  Expenses:Gebuehren:Vipps            {fee} NOK     # col 10 (positive)
  Income:Mitgliedsbeitraege          -{amount} NOK  # col 8 (negative)
```

All three must sum to zero: `net + fee - amount = 0`

### Step 5: Append to ledger

Append transactions, in date order, to the appropriate year file:
`{finance_dir}/{year}/{year}.beancount`

## Beancount format reference

All generated transactions use this structure:

```
YYYY-MM-DD * "Narration"
  key: "value"                          # metadata (4 spaces indent)
  Account:Name:Subname                 XXX.XX NOK    # posting (2 spaces indent)
```

**Key rules:**
- **Transaction line**: `date * "description"` — the `*` means the transaction is pending/cleared
- **Metadata**: indented by 2 spaces, key-value pairs with `key: "value"` syntax. Used for `quelle`, `abrechnungs_id`, `zahler`
- **Postings**: indented by 2 spaces, each starts with an account name, then the amount and currency
- **Balance**: all postings must sum to zero (double-entry bookkeeping)
- **Amounts**: always two decimal places for NOK (`196.50 NOK`, not `196.5 NOK`)
- **Income accounts**: amounts are **negative** (`-200.00 NOK`) since income decreases equity
- **Expense accounts**: amounts are **positive** (`3.50 NOK`)
- **Asset accounts**: debits (money in) are **positive** (`196.50 NOK`)

**Conventions used in this ledger:**
- Each transaction has `quelle` (source of payment: `"Vipps"` or `"Bank"`)
- Vipps transactions additionally have `abrechnungs_id` (payout batch ID) and `zahler` (payer name)
- Bank transactions (`quelle: "Bank"`) have **no fee** and **no `abrechnungs_id`** — the full amount goes to `Income:Mitgliedsbeitraege` directly
- Transactions are appended to `{year}/{year}.beancount`, which is already included from `main.beancount` via `include "{year}/{year}.beancount"`
- No blank lines between postings within a transaction; one blank line between transactions

**Vipps example:**
```beancount
2026-06-09 * "Einzelmitgliedschaft"
  quelle: "Vipps"
  abrechnungs_id: "2000370"
  zahler: "Vera Von Euw-Olsen"
  Assets:Bank:Giro                    196.50 NOK
  Expenses:Gebuehren:Vipps              3.50 NOK
  Income:Mitgliedsbeitraege          -200.00 NOK
```

**Bank example (DNB direct transfer, no Vipps):**
```beancount
2026-05-15 * "Einzelmitgliedschaft"
  quelle: "Bank"
  zahler: "Beatrice Schnyder Rånsdal"
  Assets:Bank:Giro                    200.00 NOK
  Income:Mitgliedsbeitraege          -200.00 NOK
```

For bank transactions, parse directly from DNB statements:
- **Date** = DNB booking date (col 1, `DD/MM/YYYY` → `YYYY-MM-DD`)
- **Narration** = membership type (infer from amount: 200 → `"Einzelmitgliedschaft"`, 400 → `"Familienmitgliedschaft"`)
- **zahler** = from DNB text (often in "Forklarende tekst" or extracted from Arkivref/Referanse)
- **Postings**: `Assets:Bank:Giro` (Inn amount), `Income:Mitgliedsbeitraege` (negative, same amount)
- Only one posting pair (no fee line) — no `Expenses:Gebuehren:Vipps`

## Reference: fee schedule

| Type | Amount (col 8) | Fee (col 10) | Net (col 11) |
|------|---------------|-------------|-------------|
| Einzelmitgliedschaft | 200.00 | 3.50 | 196.50 |
| Familienmitgliedschaft | 400.00 | 7.00 | 393.00 |
