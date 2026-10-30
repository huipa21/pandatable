package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/structure/structuresql"
)

func rewriteChoiceFilter(raw json.RawMessage, fieldID string, labels map[string]string) (json.RawMessage, bool, error) {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, false, fmt.Errorf("decode saved choice filter: %w", err)
	}
	if node == nil {
		return raw, false, nil
	}
	changed := false
	var currentField string
	if value := node["field"]; len(value) > 0 {
		if err := json.Unmarshal(value, &currentField); err != nil {
			return nil, false, fmt.Errorf("decode saved filter field: %w", err)
		}
	}
	if currentField == fieldID {
		if value, ok := node["value"]; ok {
			var label string
			if json.Unmarshal(value, &label) == nil {
				if replacement, found := labels[label]; found {
					encoded, err := json.Marshal(replacement)
					if err != nil {
						return nil, false, fmt.Errorf("encode renamed filter value: %w", err)
					}
					node["value"] = encoded
					changed = true
				}
			} else {
				var values []json.RawMessage
				if err := json.Unmarshal(value, &values); err == nil {
					for index, rawLabel := range values {
						var item string
						if json.Unmarshal(rawLabel, &item) != nil {
							continue
						}
						if replacement, found := labels[item]; found {
							encoded, err := json.Marshal(replacement)
							if err != nil {
								return nil, false, fmt.Errorf("encode renamed filter value: %w", err)
							}
							values[index] = encoded
							changed = true
						}
					}
					if changed {
						encoded, err := json.Marshal(values)
						if err != nil {
							return nil, false, fmt.Errorf("encode renamed filter values: %w", err)
						}
						node["value"] = encoded
					}
				}
			}
		}
	}
	if rawItems, ok := node["items"]; ok {
		var items []json.RawMessage
		if err := json.Unmarshal(rawItems, &items); err != nil {
			return nil, false, fmt.Errorf("decode saved filter group: %w", err)
		}
		for index := range items {
			next, updated, err := rewriteChoiceFilter(items[index], fieldID, labels)
			if err != nil {
				return nil, false, err
			}
			if updated {
				items[index] = next
				changed = true
			}
		}
		if changed {
			encoded, err := json.Marshal(items)
			if err != nil {
				return nil, false, fmt.Errorf("encode saved filter group: %w", err)
			}
			node["items"] = encoded
		}
	}
	if !changed {
		return raw, false, nil
	}
	encoded, err := json.Marshal(node)
	if err != nil {
		return nil, false, fmt.Errorf("encode saved choice filter: %w", err)
	}
	return encoded, true, nil
}

func rewriteChoiceConfig(raw []byte, fieldID string, labels map[string]string) ([]byte, bool, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, false, fmt.Errorf("decode saved view config: %w", err)
	}
	if config == nil {
		return nil, false, errors.New("saved view config is not an object")
	}
	changed := false
	if filter, ok := config["filter"]; ok && string(filter) != "null" {
		next, updated, err := rewriteChoiceFilter(filter, fieldID, labels)
		if err != nil {
			return nil, false, err
		}
		if updated {
			config["filter"] = next
			changed = true
		}
	}
	if rawHighlights, ok := config["highlight"]; ok {
		var highlights []json.RawMessage
		if err := json.Unmarshal(rawHighlights, &highlights); err != nil {
			return nil, false, fmt.Errorf("decode saved highlight rules: %w", err)
		}
		for index, rawRule := range highlights {
			var rule map[string]json.RawMessage
			if err := json.Unmarshal(rawRule, &rule); err != nil {
				return nil, false, fmt.Errorf("decode saved highlight rule: %w", err)
			}
			when, ok := rule["when"]
			if !ok || string(when) == "null" {
				continue
			}
			next, updated, err := rewriteChoiceFilter(when, fieldID, labels)
			if err != nil {
				return nil, false, err
			}
			if updated {
				rule["when"] = next
				highlights[index], err = json.Marshal(rule)
				if err != nil {
					return nil, false, fmt.Errorf("encode saved highlight rule: %w", err)
				}
				changed = true
			}
		}
		if changed {
			encoded, err := json.Marshal(highlights)
			if err != nil {
				return nil, false, fmt.Errorf("encode saved highlight rules: %w", err)
			}
			config["highlight"] = encoded
		}
	}
	if !changed {
		return raw, false, nil
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, false, fmt.Errorf("encode saved view config: %w", err)
	}
	return encoded, true, nil
}

func rewriteChoiceFilters(ctx context.Context, c *txn, tableID, fieldID pgtype.UUID, from, to []string) error {
	if len(from) == 0 {
		return nil
	}
	labels := make(map[string]string, len(from))
	for index := range from {
		labels[from[index]] = to[index]
	}
	views, err := c.q.ListViewConfigsForTable(ctx, tableID)
	if err != nil {
		return fmt.Errorf("list view configs for choice rename: %w", err)
	}
	for _, view := range views {
		config, changed, err := rewriteChoiceConfig(view.Config, fieldID.String(), labels)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := c.q.SaveViewConfigForTypeChange(ctx, structuresql.SaveViewConfigForTypeChangeParams{
			ID: view.ID, Config: config,
		}); err != nil {
			return fmt.Errorf("save view config for choice rename: %w", err)
		}
	}
	return nil
}
