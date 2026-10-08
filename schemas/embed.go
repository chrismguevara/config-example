// Package schemas holds the versioned JSON Schema documents for the user
// settings object. The JSON files are the source of truth shared by:
//
//   - the Go backend, which embeds them here and validates every write;
//   - the React frontend, which generates TypeScript types from the current
//     version and reads enum lists from it at runtime;
//   - the Liquibase changelog, whose data migrations rewrite stored rows
//     from version N to N+1 to match the next file.
package schemas

import "embed"

// FS contains every user-settings.v<N>.schema.json file.
//
//go:embed user-settings.v*.schema.json
var FS embed.FS

// FileName returns the schema file name for a document version.
func FileName(version int) string {
	return "user-settings.v" + itoa(version) + ".schema.json"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
