package query

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

type Params struct {
	From string
	To   string
}

func NormalizeParams(from, to string) (Params, error) {
	normalized := Params{}

	if from != "" {
		value, parsed, err := normalizeDate("from", from)
		if err != nil {
			return Params{}, err
		}

		normalized.From = value

		if to == "" {
			return normalized, nil
		}

		toValue, toParsed, err := normalizeDate("to", to)
		if err != nil {
			return Params{}, err
		}

		if parsed.After(toParsed) {
			return Params{}, fmt.Errorf("from date %s is after to date %s", value, toValue)
		}

		normalized.To = toValue
		return normalized, nil
	}

	if to != "" {
		value, _, err := normalizeDate("to", to)
		if err != nil {
			return Params{}, err
		}

		normalized.To = value
	}

	return normalized, nil
}

func RenderTemplateFile(path string, params Params) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read query template %s: %w", path, err)
	}

	tmpl, err := template.New(filepath.Base(path)).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse query template %s: %w", path, err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, params); err != nil {
		return "", fmt.Errorf("render query template %s: %w", path, err)
	}

	return out.String(), nil
}

func normalizeDate(kind, value string) (string, time.Time, error) {
	layouts := []struct {
		layout string
		kind   string
	}{
		{layout: "2006-01-02", kind: kind},
		{layout: "2006-01", kind: kind},
	}

	for _, item := range layouts {
		parsed, err := time.Parse(item.layout, value)
		if err != nil {
			continue
		}

		switch item.layout {
		case "2006-01":
			if item.kind == "from" {
				return parsed.Format("2006-01-02"), parsed, nil
			}

			endOfMonth := parsed.AddDate(0, 1, -1)
			return endOfMonth.Format("2006-01-02"), endOfMonth, nil
		default:
			return parsed.Format("2006-01-02"), parsed, nil
		}
	}

	return "", time.Time{}, fmt.Errorf("%s date %q must use YYYY-MM or YYYY-MM-DD", kind, value)
}
