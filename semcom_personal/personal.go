package personal

import (
	_ "embed"
)

// Schema is the SQL schema for the personal module.
//go:embed schema.sql
var Schema string
