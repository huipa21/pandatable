package web

import (
	"embed"
	"io/fs"
)

// all: keeps clean checkouts embeddable via .gitkeep before the web build.
//
//go:embed all:dist
var assets embed.FS

func Assets() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
