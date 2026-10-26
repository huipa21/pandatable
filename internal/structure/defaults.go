package structure

import (
	"encoding/json"
	"fmt"
)

// labelSet holds the display names written once into metadata when a table is
// created, in the creator's profile language (D-098). They are ordinary
// renamable display names; physical names never derive from them.
type labelSet struct {
	name, rowID, createdAt, createdBy, updatedAt, updatedBy, grid string
}

var labelSets = map[string]labelSet{
	"en":    {"Name", "ID", "Created at", "Created by", "Updated at", "Updated by", "Grid"},
	"zh-CN": {"名称", "ID", "创建时间", "创建人", "更新时间", "更新人", "表格"},
}

func labelsFor(locale string) labelSet {
	if labels, ok := labelSets[locale]; ok {
		return labels
	}
	return labelSets["en"]
}

const (
	nameFieldPosition   = positionStep
	systemFieldPosition = 1e9
	gridNameWidth       = 240
)

// systemField describes one of the five physical columns ddl.CreateTable
// installs (D-025). UI kinds equal the column names and are read-only.
type systemField struct {
	column, pgType string
	required       bool
	label          func(labelSet) string
}

var systemFields = []systemField{
	{"id", "bigint", true, func(l labelSet) string { return l.rowID }},
	{"created_at", "timestamptz", true, func(l labelSet) string { return l.createdAt }},
	{"created_by", "uuid", false, func(l labelSet) string { return l.createdBy }},
	{"updated_at", "timestamptz", true, func(l labelSet) string { return l.updatedAt }},
	{"updated_by", "uuid", false, func(l labelSet) string { return l.updatedBy }},
}

type gridColumn struct {
	ID      string `json:"id"`
	Visible bool   `json:"visible"`
	Width   int    `json:"width,omitempty"`
}

type gridConfig struct {
	Fields    []gridColumn `json:"fields"`
	Frozen    int          `json:"frozen"`
	RowHeight string       `json:"rowHeight"`
}

// defaultGridConfig lists the display field first (visible, frozen) followed by
// the hidden system fields, following the doc 08 view-config shape.
func defaultGridConfig(displayFieldID string, systemFieldIDs []string) ([]byte, error) {
	config := gridConfig{Fields: []gridColumn{{ID: displayFieldID, Visible: true, Width: gridNameWidth}},
		Frozen: 1, RowHeight: "short"}
	for _, id := range systemFieldIDs {
		config.Fields = append(config.Fields, gridColumn{ID: id})
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode default grid config: %w", err)
	}
	return encoded, nil
}
