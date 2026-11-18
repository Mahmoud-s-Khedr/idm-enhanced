// Package extensions exposes the canonical development extension identities.
package extensions

import _ "embed"

//go:embed identities.json
var Identities []byte
