package core

import "testing"

func TestNamesUnknownName(t *testing.T) {
	cases := []struct {
		text        string
		displayName string
		unknownName string
		want        bool
	}{
		{"EDGAR VAEL arrives", "The Tinker", "Edgar Vael", true},
		{"Edgar Vael's role", "The Tinker", "Edgar Vael", true},
		{"Edgar\u00a0Vael arrives", "The Tinker", "Edgar Vael", true},
		{"Edgar  Vael arrives", "The Tinker", "Edgar Vael", true},
		{"Edgar\nVael arrives", "The Tinker", "Edgar Vael", true},
		{"Edgar\u200bVael arrives", "The Tinker", "Edgar Vael", true},
		{"Edgar\u200b Vael arrives", "The Tinker", "Edgar Vael", true},
		{"The annual harvest, always", "Anna", "Ann", false},
		{"Anna joined the colony", "Anna", "Ann", false},
		{"Ann joined the colony", "Anna", "Ann", true},
		{"Old Tam joined the colony", "Old Tam", "Tam", false},
		{"Old Tam and Tam", "Old Tam", "Tam", true},
		{"Tam Grey joined the colony", "Tam", "Tam Grey", true},
		{"The Tinker joined the colony", "The Tinker", "Edgar Vael", false},
	}
	for _, testCase := range cases {
		if got := namesUnknownName(testCase.text, testCase.displayName, testCase.unknownName); got != testCase.want {
			t.Errorf("namesUnknownName(%q, %q, %q) = %v, want %v", testCase.text, testCase.displayName, testCase.unknownName, got, testCase.want)
		}
	}
}
