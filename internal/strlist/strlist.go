// Package strlist holds helpers for the string lists used by Lexeme
// definitions and Translations.
package strlist

import "strings"

// MergeUnique concatenates lists, trimming whitespace, dropping blank items and
// keeping only the first occurrence of each item compared case-insensitively.
// The result is never nil.
func MergeUnique(lists ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, list := range lists {
		for _, item := range list {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}
