package structure

import (
	"reflect"
	"testing"
)

func TestGalleryConfigCleanupAndPackageRemapping(t *testing.T) {
	cover := "photo"
	config := GalleryConfig{CoverField: &cover, Fields: []string{"title", "removed", "title", "photo"}}
	fields := []Field{
		{ID: "title", UIType: "text"},
		{ID: "photo", UIType: "attachment"},
	}
	clean := normalizeGalleryConfig(config, fields)
	if clean.CoverField == nil || *clean.CoverField != cover ||
		!reflect.DeepEqual(clean.Fields, []string{"title", "photo"}) {
		t.Fatalf("clean Gallery config = %+v", clean)
	}
	fields[1].UIType = "text"
	if got := normalizeGalleryConfig(config, fields); got.CoverField != nil {
		t.Fatalf("changed field type retained cover: %+v", got)
	}
	source := ViewColumns{
		Fields:  []ViewColumn{{ID: "title", Visible: true}, {ID: "photo", Visible: true}},
		Gallery: &clean,
	}
	mapped, err := packageViewConfig(source, map[string]string{"title": "f1", "photo": "f2"})
	if err != nil || mapped.Gallery == nil || *mapped.Gallery.CoverField != "f2" ||
		!reflect.DeepEqual(mapped.Gallery.Fields, []string{"f1", "f2"}) {
		t.Fatalf("mapped Gallery config = %+v, %v", mapped.Gallery, err)
	}
	restored, err := packageViewConfig(mapped, map[string]string{"f1": "title", "f2": "photo"})
	if err != nil || !reflect.DeepEqual(restored.Gallery, &clean) {
		t.Fatalf("restored Gallery config = %+v, %v", restored.Gallery, err)
	}
	if _, err := packageViewConfig(source, map[string]string{"title": "f1"}); err == nil {
		t.Fatal("package export accepted an unavailable Gallery field")
	}
}
