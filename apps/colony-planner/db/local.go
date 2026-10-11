package db

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// RequireLocalHost refuses a connection string unless every host pgx would try is localhost or
// 127.0.0.1. It reads the string exactly as pgx does, because pgx lets a `host` query parameter, a
// `service` entry or a second host in the list override the address the URL appears to name; checking
// the URL's own host would let `colony reset` drop a remote schema.
func RequireLocalHost(connectionString string) error {
	config, err := pgconn.ParseConfig(connectionString)
	if err != nil {
		return fmt.Errorf("the database address can't be read: %w", err)
	}
	hosts := []string{config.Host}
	for _, fallback := range config.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	for _, host := range hosts {
		if host != "localhost" && host != "127.0.0.1" {
			return fmt.Errorf("refusing to run against %q: the database must be on localhost or 127.0.0.1", host)
		}
	}
	return nil
}
