package web

import (
	"io/fs"
	"testing"
)

func TestAssetsRoot(t *testing.T) {
	assets, err := Assets()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadDir(assets, "."); err != nil {
		t.Fatalf("embedded dist root is unavailable: %v", err)
	}
}
