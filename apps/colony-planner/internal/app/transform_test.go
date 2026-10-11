package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

func TestRedactionTransformer(t *testing.T) {
	member := core.Member{
		ID:          uuid.New(),
		DisplayName: "Ana",
		Dm:          &core.MemberDm{DiscordUserID: "seed:ana", Status: store.MemberStatusACTIVE},
	}
	operation := &huma.Operation{Method: http.MethodGet, Path: "/api/test"}

	for _, testCase := range []struct {
		name    string
		viewer  *core.Viewer
		keepsDm bool
	}{
		{name: "visitor", viewer: nil, keepsDm: false},
		{name: "player", viewer: &core.Viewer{Status: store.MemberStatusACTIVE}, keepsDm: false},
		{name: "DM viewing as player", viewer: &core.Viewer{Status: store.MemberStatusACTIVE, IsDM: true, ViewAsPlayer: true}, keepsDm: false},
		{name: "DM", viewer: &core.Viewer{Status: store.MemberStatusACTIVE, IsDM: true}, keepsDm: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if testCase.viewer != nil {
				request = request.WithContext(auth.WithViewer(request.Context(), *testCase.viewer))
			}
			humaContext := humatest.NewContext(operation, request, httptest.NewRecorder())

			transformed, err := redactDmFields(humaContext, "200", member)
			if err != nil {
				t.Fatalf("transform: %v", err)
			}
			result, ok := transformed.(core.Member)
			if !ok {
				t.Fatalf("transform returned %T, want core.Member", transformed)
			}
			if (result.Dm != nil) != testCase.keepsDm {
				t.Fatalf("dm present = %v, want %v", result.Dm != nil, testCase.keepsDm)
			}
			if result.DisplayName != "Ana" {
				t.Fatalf("visible field lost: %+v", result)
			}
		})
	}
	if member.Dm == nil {
		t.Fatal("the transformer modified the handler's value")
	}
}
