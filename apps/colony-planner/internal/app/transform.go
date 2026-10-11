package app

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// redactDmFields is the one response Transformer: every response body loses its `dm` objects unless
// the viewer is the DM and not viewing as a player. No handler redacts anything itself.
func redactDmFields(humaContext huma.Context, _ string, body any) (any, error) {
	viewer, signedIn := auth.ViewerFromContext(humaContext.Context())
	if signedIn && viewer.SeesDmFields() {
		return body, nil
	}
	return core.RedactDm(body), nil
}
