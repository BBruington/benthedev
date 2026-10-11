// Command colony is the whole Colony Planner service in one binary.
//
//	colony serve              run the web server (migrations are applied first) and Beny Bot
//	colony migrate up|status  apply or list migrations
//	colony openapi            print the OpenAPI document (mise run gen)
//	colony seed [--hexes N]   load the fictional sample campaign into a fresh local database
//	colony reset              drop everything in the local database and re-run the migrations
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	// Settings validate IANA zones, and a distroless image (M6) has no zoneinfo of its own.
	_ "time/tzdata"
)

const usage = `usage: colony <command>

commands:
  serve              run the web server (applies migrations first) and Beny Bot
  migrate up|status  apply or list database migrations
  openapi            print the OpenAPI document to stdout
  seed [--hexes N]   load the sample campaign into a freshly migrated local database
  reset              drop everything in the local database and re-run the migrations`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "colony:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("missing command\n\n%s", usage)
	}
	commandContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command, commandArguments := arguments[0], arguments[1:]
	switch command {
	case "serve":
		return serve(commandContext)
	case "migrate":
		return migrate(commandContext, commandArguments)
	case "openapi":
		return printOpenAPI(os.Stdout)
	case "seed":
		return seedDatabase(commandContext, commandArguments)
	case "reset":
		return resetDatabase(commandContext)
	case "help", "-h", "--help":
		fmt.Println(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, usage)
	}
}
