package fava

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ReportKind names a Fava JSON API tree report endpoint.
type ReportKind string

const (
	KindBalanceSheet    ReportKind = "balance_sheet"
	KindIncomeStatement ReportKind = "income_statement"
	KindTrialBalance    ReportKind = "trial_balance"
)

// Client talks to the JSON API of a running Fava instance.
//
// Fava resolves the ledger via a "bfile" slug in the URL. The slug is either
// configured upfront (SetSlug) or discovered from Fava's index redirect.
type Client struct {
	baseURL string
	slug    string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// SetSlug pins the bfile slug, skipping automatic discovery.
func (c *Client) SetSlug(slug string) {
	c.slug = slug
}

// Slug returns the resolved bfile slug, discovering it if needed.
func (c *Client) Slug(ctx context.Context) (string, error) {
	if c.slug != "" {
		return c.slug, nil
	}
	if _, err := c.DiscoverSlug(ctx); err != nil {
		return "", err
	}
	return c.slug, nil
}

// DiscoverSlug follows Fava's index redirect (GET / -> /<bfile>/<report>/)
// and extracts the bfile slug from the Location header.
func (c *Client) DiscoverSlug(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/", nil)
	if err != nil {
		return "", fmt.Errorf("build index request: %w", err)
	}

	client := &http.Client{
		Timeout: c.http.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("reach fava at %s: %w", c.baseURL, err)
	}
	defer response.Body.Close()

	location := response.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("fava at %s returned status %d without a bfile redirect", c.baseURL, response.StatusCode)
	}

	slug := extractSlug(location)
	if slug == "" {
		return "", fmt.Errorf("could not extract bfile slug from redirect %q", location)
	}

	c.slug = slug
	return slug, nil
}

// TreeReport fetches one of Fava's hierarchical tree reports, optionally
// restricted by Fava's "time" filter expression (e.g. "2026", "2026-12-31").
func (c *Client) TreeReport(ctx context.Context, kind ReportKind, timeFilter string) (*TreeReport, error) {
	slug, err := c.Slug(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/%s/api/%s", c.baseURL, slug, kind)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("build %s url: %w", kind, err)
	}

	query := parsed.Query()
	if timeFilter != "" {
		query.Set("time", timeFilter)
	}
	parsed.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", kind, err)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", kind, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("fetch %s: status %d: %s", kind, response.StatusCode, strings.TrimSpace(string(body)))
	}

	var envelope struct {
		Data  TreeReport `json:"data"`
		Mtime string     `json:"mtime"`
	}

	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", kind, err)
	}

	return &envelope.Data, nil
}

// LedgerData fetches Fava's ledger metadata (title, account names, options).
func (c *Client) LedgerData(ctx context.Context) (*LedgerData, error) {
	slug, err := c.Slug(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/%s/api/ledger_data", c.baseURL, slug)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build ledger_data request: %w", err)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch ledger_data: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch ledger_data: status %d", response.StatusCode)
	}

	var envelope struct {
		Data LedgerData `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode ledger_data response: %w", err)
	}

	return &envelope.Data, nil
}

// TreeReport is Fava's hierarchical tree report response.
type TreeReport struct {
	Trees []TreeNode `json:"trees"`
}

// TreeNode is a single account node in a TreeReport.
type TreeNode struct {
	Account         string `json:"account"`
	Balance         Amount `json:"balance"`
	BalanceChildren Amount `json:"balance_children"`
	Children        []TreeNode `json:"children"`
	HasTxns         bool   `json:"has_txns"`
}

// Amount maps a currency to its decimal value. Fava serializes decimal values
// as JSON numbers or strings depending on the version, so values are kept as
// raw strings and accepted in both forms.
type Amount map[string]Decimal

// Decimal is a decimal amount kept as its raw string representation.
type Decimal string

// UnmarshalJSON accepts both a JSON number (123.45) and a JSON string
// ("123.45") for a decimal amount.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	*d = Decimal(value)
	return nil
}

// LedgerData holds the metadata Fava reports about the loaded ledger.
type LedgerData struct {
	Options struct {
		Title           string `json:"title"`
		NameAssets      string `json:"name_assets"`
		NameLiabilities string `json:"name_liabilities"`
		NameEquity      string `json:"name_equity"`
		NameIncome      string `json:"name_income"`
		NameExpenses    string `json:"name_expenses"`
	} `json:"options"`
}

func extractSlug(location string) string {
	parsed, err := url.Parse(location)
	if err != nil {
		return ""
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}

	return parts[0]
}