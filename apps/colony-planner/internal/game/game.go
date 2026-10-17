// Package game holds the settled colony rules as pure, deterministic functions with table tests.
// It does no I/O: no database, no network, no time.Now and no randomness, so every rule can be tested
// with the spec's worked examples verbatim. golangci-lint's depguard and forbidigo enforce this.
package game
