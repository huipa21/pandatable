package web

import (
	"embed"
	"io/fs"
)

// all: keeps the pre-T0.5 placeholder embeddable without shipping a fake SPA.
//
//go:embed all:dist
var assets embed.FS

func Assets() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
