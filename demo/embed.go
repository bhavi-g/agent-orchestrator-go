package demo

import "embed"

//go:embed logs/*
var Logs embed.FS
