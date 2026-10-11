package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const canary = "CANARY-dm-secret"

type secretDetails struct {
	TrueName string `json:"trueName"`
}

type redactionLeaf struct {
	Name string         `json:"name"`
	Dm   *secretDetails `json:"dm,omitempty"`
}

type redactionRoot struct {
	Title     string                   `json:"title"`
	When      time.Time                `json:"when"`
	Dm        *secretDetails           `json:"dm,omitempty"`
	Leaf      redactionLeaf            `json:"leaf"`
	LeafRef   *redactionLeaf           `json:"leafRef"`
	Leaves    []redactionLeaf          `json:"leaves"`
	ByKey     map[string]redactionLeaf `json:"byKey"`
	Loose     map[string]any           `json:"loose"`
	Anything  any                      `json:"anything"`
	Fixed     [2]redactionLeaf         `json:"fixed"`
	EmptyList []redactionLeaf          `json:"emptyList"`
}

func secret() *secretDetails {
	return &secretDetails{TrueName: canary}
}

func sampleRoot() redactionRoot {
	return redactionRoot{
		Title:   "visible",
		When:    time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
		Dm:      secret(),
		Leaf:    redactionLeaf{Name: "leaf", Dm: secret()},
		LeafRef: &redactionLeaf{Name: "pointer leaf", Dm: secret()},
		Leaves:  []redactionLeaf{{Name: "first", Dm: secret()}, {Name: "second", Dm: secret()}},
		ByKey:   map[string]redactionLeaf{"one": {Name: "mapped", Dm: secret()}},
		Loose: map[string]any{
			"dm":     canary,
			"nested": map[string]any{"dm": map[string]any{"trueName": canary}, "keep": "yes"},
		},
		Anything: &redactionLeaf{Name: "boxed", Dm: secret()},
		Fixed:    [2]redactionLeaf{{Name: "array", Dm: secret()}, {Name: "array two"}},
	}
}

func TestRedactDmRemovesEveryDmObject(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input any
	}{
		{name: "struct value", input: sampleRoot()},
		{name: "pointer", input: new(sampleRoot())},
		{name: "slice", input: []redactionRoot{sampleRoot(), sampleRoot()}},
		{name: "map", input: map[string]*redactionRoot{"root": new(sampleRoot())}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(RedactDm(testCase.input))
			if err != nil {
				t.Fatalf("marshal redacted value: %v", err)
			}
			if strings.Contains(string(encoded), canary) {
				t.Fatalf("redacted output still contains the canary: %s", encoded)
			}
			if strings.Contains(string(encoded), `"dm"`) {
				t.Fatalf("redacted output still contains a dm key: %s", encoded)
			}
			for _, visible := range []string{"visible", "pointer leaf", "second", "mapped", "boxed", "array two", `"keep":"yes"`, "2026-10-10T12:00:00Z"} {
				if !strings.Contains(string(encoded), visible) {
					t.Fatalf("redacted output lost visible data %q: %s", visible, encoded)
				}
			}
		})
	}
}

func TestRedactDmLeavesTheOriginalUntouched(t *testing.T) {
	original := sampleRoot()
	RedactDm(original)
	RedactDm(&original)

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	if count := strings.Count(string(encoded), canary); count != 10 {
		t.Fatalf("original lost DM data: found %d canaries in %s", count, encoded)
	}
}

func TestRedactDmHandlesNil(t *testing.T) {
	if RedactDm(nil) != nil {
		t.Fatal("RedactDm(nil) should be nil")
	}
	var missing *redactionRoot
	if result, ok := RedactDm(missing).(*redactionRoot); !ok || result != nil {
		t.Fatalf("RedactDm of a nil pointer should stay a nil pointer, got %#v", result)
	}
}
