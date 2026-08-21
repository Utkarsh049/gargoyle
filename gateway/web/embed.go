package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/* templates/*
var embeddedFiles embed.FS

// StaticFS returns the sub-filesystem for static assets (/static/...).
func StaticFS() http.Handler {
	sub, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

// TemplatesFS returns the raw embedded filesystem containing templates.
func TemplatesFS() embed.FS {
	return embeddedFiles
}
