//go:build !dev
// +build !dev

package main

import "embed"

//go:embed all:frontend/dist
var embeddedFiles embed.FS
