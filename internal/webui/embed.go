package webui

import "embed"

// DistFS contains the built frontend assets.
// The Dockerfile and Makefile ensure web/dist is populated before go build.
// In development, this will contain only .gitkeep; the Vite dev server is used instead.
//
//go:embed all:dist
var DistFS embed.FS
