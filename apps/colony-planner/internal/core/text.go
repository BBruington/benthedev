package core

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// cleanText trims free text from a person and checks it is plain, one-line text of at most maxLength
// characters. Every free-text input goes through it: Postgres refuses U+0000 outright (a 500 instead
// of a message), and tabs, line breaks and other control characters have no place in a name. A blank
// result is the caller's to judge. The FieldError's message is the one the person sees.
func cleanText(field, value string, maxLength int) (string, *FieldError) {
	trimmed := strings.TrimSpace(value)
	if strings.ContainsFunc(trimmed, unicode.IsControl) {
		return "", &FieldError{Field: field, Message: "Remove the tabs, line breaks or other control characters."}
	}
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", &FieldError{Field: field, Message: fmt.Sprintf("Use at most %d characters.", maxLength)}
	}
	return trimmed, nil
}

// cleanMultilineText is cleanText for longer prose (a timeline entry's body): line breaks are kept,
// normalized to \n, and every other control character, NUL included, is refused.
func cleanMultilineText(field, value string, maxLength int) (string, *FieldError) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	trimmed := strings.TrimSpace(normalized)
	if strings.ContainsFunc(trimmed, func(character rune) bool { return character != '\n' && unicode.IsControl(character) }) {
		return "", &FieldError{Field: field, Message: "Remove the tabs or other control characters."}
	}
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", &FieldError{Field: field, Message: fmt.Sprintf("Use at most %d characters.", maxLength)}
	}
	return trimmed, nil
}
