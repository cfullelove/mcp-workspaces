//go:build dev
// +build dev

package main

import "embed"

// In dev mode, we don't embed anything.
var embeddedFiles embed.FS
