package structure

import (
	"encoding/json"
	"errors"
	"fmt"
)

func fieldOptionsForCodec(raw []byte) ([]byte, error) {
	var options map[string]json.RawMessage
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, fmt.Errorf("decode field options: %w", err)
	}
	if options == nil {
		return nil, errors.New("field options are not an object")
	}
	delete(options, "indexed")
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("encode codec field options: %w", err)
	}
	return encoded, nil
}

func fieldOptionsForResponse(raw []byte) ([]byte, error) {
	return fieldOptionsForCodec(raw)
}

func mergeAutomaticIndexMetadata(options, current []byte) ([]byte, error) {
	var updated, stored map[string]json.RawMessage
	if err := json.Unmarshal(options, &updated); err != nil {
		return nil, fmt.Errorf("decode updated field options: %w", err)
	}
	if updated == nil {
		return nil, errors.New("updated field options are not an object")
	}
	if err := json.Unmarshal(current, &stored); err != nil {
		return nil, fmt.Errorf("decode stored field options: %w", err)
	}
	if stored == nil {
		return nil, errors.New("stored field options are not an object")
	}
	if indexed, exists := stored["indexed"]; exists {
		var values map[string]bool
		if err := json.Unmarshal(indexed, &values); err != nil || values == nil {
			return nil, errors.New("stored indexed option is not an object")
		}
		updated["indexed"] = indexed
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		return nil, fmt.Errorf("encode field options with automatic-index metadata: %w", err)
	}
	return encoded, nil
}
