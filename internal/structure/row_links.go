package structure

import (
	"bytes"
	"encoding/json"
	"sort"

	"pandatable/internal/apperr"
)

func patchLinkIDs(raw json.RawMessage) ([]int64, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		return nil, validation("Relation values must be an array of linked rows")
	}
	var links []LinkValue
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, validation("Relation values must be an array of linked rows")
	}
	ids := make([]string, len(links))
	for index, link := range links {
		ids[index] = link.ID
	}
	result, err := relationIDs(ids)
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func equalLinkIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func linkIDDifference(left, right []int64) []int64 {
	result := make([]int64, 0)
	for _, id := range left {
		if !containsID(right, id) {
			result = append(result, id)
		}
	}
	return result
}

func undoConflict() error {
	return apperr.New(409, "ERR_UNDO_CONFLICT", "The row changed; review and retry")
}

func validatePatchedLinks(table rowTable, row RowRecord, input map[string]json.RawMessage) error {
	for fieldID, raw := range input {
		if field := table.byID[fieldID]; field.relation != nil {
			want, err := patchLinkIDs(raw)
			if err != nil {
				return err
			}
			actual, err := patchLinkIDs(row.Fields[fieldID])
			if err != nil {
				return err
			}
			if !equalLinkIDs(want, actual) {
				return validation("Patched relation fields have conflicting linked rows")
			}
		}
	}
	return nil
}
