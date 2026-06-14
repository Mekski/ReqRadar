// Package entity holds entity-resolution helpers shared by the seeder and the
// processor's resolution cascade, so normalization is defined exactly once.
package entity

import "strings"

// Normalize canonicalizes a raw entity string for alias matching: lowercase and
// collapse internal whitespace. Suffix/punctuation stripping is added together
// with the resolution cascade in its own task, under test — keeping this minimal
// for now avoids seeding aliases that collide before the resolver can handle them.
func Normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
