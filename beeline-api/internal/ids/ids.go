// Package ids generates prefixed, unguessable resource identifiers such as "pay_…".
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// New returns prefix + "_" + 26 lowercase base32 characters (128 bits of randomness).
func New(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return prefix + "_" + strings.ToLower(enc.EncodeToString(b[:]))
}

const (
	Workspace = "ws"
	APIKey    = "key"
	Quote     = "qte"
	Payment   = "pay"
	Event     = "evt"
	Invoice   = "inv"
	Attempt   = "att"
)
