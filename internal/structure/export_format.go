package structure

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"pandatable/internal/codec"
	"pandatable/internal/formula"
)

func (plan *exportPlan) writeCSV(ctx context.Context, w io.Writer) error {
	if _, err := w.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return fmt.Errorf("write CSV BOM: %w", err)
	}
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	header := make([]string, len(plan.fields))
	for index, field := range plan.fields {
		header[index] = field.name
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}
	err := plan.eachBatch(ctx, func(rows []exportRow, labels map[string]string) error {
		for _, row := range rows {
			values := make([]string, len(plan.fields))
			for index, field := range plan.fields {
				text, err := plan.formatExportCell(row, field, labels)
				if err != nil {
					return err
				}
				values[index] = text
			}
			if err := writer.Write(values); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
		}
		writer.Flush()
		return writer.Error()
	})
	if err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush CSV output: %w", err)
	}
	return nil
}

func (plan *exportPlan) formatExportCell(row exportRow, field rowField, labels map[string]string) (string, error) {
	raw := row.record.Fields[field.id]
	if field.codec != nil {
		text, err := field.codec.FormatText(row.values[field.id], codec.Env{Location: plan.location})
		if err != nil {
			return "", fmt.Errorf("format export field: %w", err)
		}
		switch field.kind {
		case codec.User, codec.MultiUser, codec.CreatedBy, codec.UpdatedBy:
			ids, err := exportUserIDs(raw)
			if err != nil {
				return "", err
			}
			names := make([]string, len(ids))
			for index, id := range ids {
				if name, ok := labels[id]; ok {
					names[index] = name
				} else {
					names[index] = id
				}
			}
			return strings.Join(names, ", "), nil
		case codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
			if isJSONNull(raw) {
				return "", nil
			}
			instant, err := exportInstant(raw)
			if err != nil {
				return "", err
			}
			return instant.In(plan.location).Format("2006-01-02 15:04:05"), nil
		}
		return text, nil
	}
	if field.relation != nil {
		if isJSONNull(raw) || len(raw) == 0 {
			return "", nil
		}
		labels, err := exportLinkLabels(raw)
		if err != nil {
			return "", err
		}
		return strings.Join(labels, ", "), nil
	}
	if (field.formula != nil || field.computed != nil) && formulaFieldType(field) == formula.DateTime &&
		len(raw) > 0 && !isJSONNull(raw) {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil && text != "#ERROR" && text != "🔒" {
			instant, err := exportInstant(raw)
			if err != nil {
				return "", err
			}
			return instant.In(plan.location).Format("2006-01-02 15:04:05"), nil
		}
	}
	return formatExportJSON(raw)
}

func exportLinkLabels(raw json.RawMessage) ([]string, error) {
	var links []LinkValue
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, errors.New("export link value is invalid")
	}
	labels := make([]string, len(links))
	for index, link := range links {
		labels[index] = link.Label
	}
	return labels, nil
}

func exportInstant(raw json.RawMessage) (time.Time, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return time.Time{}, errors.New("export datetime has an invalid shape")
	}
	instant, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse export datetime: %w", err)
	}
	return instant, nil
}

func formatExportJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode computed export value: %w", err)
	}
	return formatExportValue(value)
}

func formatExportValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case json.Number:
		return typed.String(), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case []any:
		values := make([]string, len(typed))
		for index, item := range typed {
			text, err := formatExportValue(item)
			if err != nil {
				return "", err
			}
			values[index] = text
		}
		return strings.Join(values, ", "), nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("encode computed export value: %w", err)
		}
		return string(encoded), nil
	}
}

func (plan *exportPlan) spreadsheetValue(row exportRow, field rowField, labels map[string]string) (any, error) {
	raw := row.record.Fields[field.id]
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	text, err := plan.formatExportCell(row, field, labels)
	if err != nil {
		return nil, err
	}
	switch field.kind {
	case codec.Checkbox:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, errors.New("export checkbox value is invalid")
		}
		return value, nil
	case codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent, codec.Rating, codec.RowID:
		return spreadsheetNumber(text)
	case codec.AutoNumber:
		if number, err := spreadsheetNumber(text); err == nil && !hasLeadingZeroes(text) {
			return number, nil
		}
	case codec.MultiSelect, codec.TextArray, codec.IntArray, codec.MultiUser:
		var values []string
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, errors.New("export list value has an invalid shape")
		}
		text = string(raw)
	case codec.Attachment:
		var references []XLSXAttachmentReference
		if err := json.Unmarshal(raw, &references); err != nil || references == nil {
			return nil, errors.New("export attachment value has an invalid shape")
		}
		encoded, err := json.Marshal(references)
		if err != nil {
			return nil, fmt.Errorf("encode export attachment references: %w", err)
		}
		text = string(encoded)
	case codec.User, codec.CreatedBy, codec.UpdatedBy:
		text, err = field.codec.FormatText(row.values[field.id], codec.Env{Location: plan.location})
		if err != nil {
			return nil, fmt.Errorf("format export user identity: %w", err)
		}
	case codec.Date:
		value, err := time.Parse("2006-01-02", text)
		if err != nil {
			return nil, fmt.Errorf("parse export date: %w", err)
		}
		return spreadsheetDate{value: value}, nil
	case codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
		value, err := exportInstant(raw)
		if err != nil {
			return nil, err
		}
		return spreadsheetDate{value: value.UTC(), dateTime: true}, nil
	case codec.Time:
		value, err := time.Parse("15:04:05", text)
		if err != nil {
			return nil, fmt.Errorf("parse export time: %w", err)
		}
		elapsed := time.Duration(value.Hour())*time.Hour + time.Duration(value.Minute())*time.Minute +
			time.Duration(value.Second())*time.Second + time.Duration(value.Nanosecond())
		return float64(elapsed) / float64(24*time.Hour), nil
	}
	if (field.formula != nil || field.computed != nil) && text != "#ERROR" && text != "🔒" {
		switch formulaFieldType(field) {
		case formula.Number:
			return spreadsheetNumber(text)
		case formula.Boolean:
			value, err := strconv.ParseBool(text)
			if err != nil {
				return nil, fmt.Errorf("parse formula boolean: %w", err)
			}
			return value, nil
		case formula.Date, formula.DateTime:
			layout := "2006-01-02"
			dateTime := formulaFieldType(field) == formula.DateTime
			if dateTime {
				value, err := exportInstant(raw)
				if err != nil {
					return nil, err
				}
				return spreadsheetDate{value: value.UTC(), dateTime: true}, nil
			}
			value, err := time.Parse(layout, text)
			if err != nil {
				return nil, fmt.Errorf("parse formula date: %w", err)
			}
			return spreadsheetDate{value: value, dateTime: dateTime}, nil
		}
	}
	if field.relation != nil {
		labels, err := exportLinkLabels(raw)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(labels)
		if err != nil {
			return nil, fmt.Errorf("encode export link labels: %w", err)
		}
		text = string(encoded)
	}
	if len(utf16.Encode([]rune(text))) > maxSpreadsheetCellChars {
		return nil, validation("A cell is too long for an XLSX package")
	}
	return text, nil
}

func hasLeadingZeroes(text string) bool {
	digits := strings.TrimPrefix(strings.TrimPrefix(text, "-"), "+")
	return len(digits) > 1 && digits[0] == '0' && digits[1] >= '0' && digits[1] <= '9'
}

type spreadsheetDate struct {
	value    time.Time
	dateTime bool
}

func spreadsheetNumber(text string) (any, error) {
	trimmed := strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(text, "-"), "+"), "0.")
	significant := 0
	for _, digit := range trimmed {
		if digit >= '0' && digit <= '9' {
			significant++
		} else if digit == 'e' || digit == 'E' {
			break
		} else if digit != '.' {
			return nil, errors.New("export numeric value is invalid")
		}
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("parse export number: %w", err)
	}
	if significant > 15 {
		return text, nil
	}
	return number, nil
}
