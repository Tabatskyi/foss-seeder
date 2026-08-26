package web

import "embed"

//go:embed templates/* templates/partials/* static/* static/css/* static/js/*
var Assets embed.FS
