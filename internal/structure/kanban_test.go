package structure

import (
	"reflect"
	"testing"
)

func TestKanbanConfigCleanupAndPackageRemapping(t *testing.T) {
	group := "status"
	config := KanbanConfig{GroupField: &group, Fields: []string{"title", "removed", "title", "status"}}
	fields := []Field{
		{ID: "title", UIType: "text"},
		{ID: "status", UIType: "single_select"},
	}
	clean := normalizeKanbanConfig(config, fields)
	if clean.GroupField == nil || *clean.GroupField != group ||
		!reflect.DeepEqual(clean.Fields, []string{"title", "status"}) {
		t.Fatalf("clean Kanban config = %+v", clean)
	}
	fields[1].UIType = "text"
	if got := normalizeKanbanConfig(config, fields); got.GroupField != nil {
		t.Fatalf("changed field type retained grouping: %+v", got)
	}
	fields[1].UIType = "single_select"
	source := ViewColumns{
		Fields: []ViewColumn{{ID: "title", Visible: true}, {ID: "status", Visible: true}},
		Kanban: &clean,
	}
	mapped, err := packageViewConfig(source, map[string]string{"title": "f1", "status": "f2"})
	if err != nil || mapped.Kanban == nil || *mapped.Kanban.GroupField != "f2" ||
		!reflect.DeepEqual(mapped.Kanban.Fields, []string{"f1", "f2"}) {
		t.Fatalf("mapped Kanban config = %+v, %v", mapped.Kanban, err)
	}
	restored, err := packageViewConfig(mapped, map[string]string{"f1": "title", "f2": "status"})
	if err != nil || !reflect.DeepEqual(restored.Kanban, &clean) {
		t.Fatalf("restored Kanban config = %+v, %v", restored.Kanban, err)
	}
	if _, err := packageViewConfig(source, map[string]string{"title": "f1"}); err == nil {
		t.Fatal("package export accepted an unavailable Kanban field")
	}
}
