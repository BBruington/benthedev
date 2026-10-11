package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/BBruington/colony-planner/internal/app"
	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// printOpenAPI builds the API without a database and prints its document, indented so diffs of the
// committed web/src/api/openapi.json stay readable.
func printOpenAPI(output io.Writer) error {
	authService, err := auth.New(auth.Options{AppEnvironment: "development"})
	if err != nil {
		return err
	}
	server, err := app.New(app.Options{Core: core.New(core.Options{}), Auth: authService})
	if err != nil {
		return err
	}
	document, err := json.MarshalIndent(server.OpenAPI(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode the OpenAPI document: %w", err)
	}
	_, err = fmt.Fprintf(output, "%s\n", document)
	return err
}
