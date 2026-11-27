package schema

import (
	"regexp"
	"strings"
)

// Normalization only decides equality; DDL always uses the original text.

var (
	spaces       = regexp.MustCompile(`\s+`)
	intWidth     = regexp.MustCompile(`^(tinyint|smallint|mediumint|int|integer|bigint)\(\d+\)`)
	castTypeName = regexp.MustCompile(`^[a-z_][a-z0-9_ ]*(\([0-9, ]*\))?(\[\])*$`)
)

// normalizeType lowercases a declared type, collapses spaces and drops the
// MySQL integer display width, which MySQL 8 no longer reports.
func normalizeType(t string) string {
	t = strings.ToLower(spaces.ReplaceAllString(strings.TrimSpace(t), " "))
	return intWidth.ReplaceAllString(t, "$1")
}

// normalizeDefault makes equivalent default expressions compare equal:
// outer parentheses (SQL Server), trailing casts ('a'::character varying in
// PostgreSQL), letter case outside string literals, and
// current_timestamp() (MariaDB) vs current_timestamp (MySQL).
func normalizeDefault(e string) string {
	e = lowerOutsideQuotes(strings.TrimSpace(e))
	for {
		before := e
		e = unwrapOuterParens(e)
		e = stripTrailingCast(e)
		if e == before {
			break
		}
	}
	return strings.ReplaceAll(e, "current_timestamp()", "current_timestamp")
}

func lowerOutsideQuotes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inQuote := false
	for _, r := range s {
		if r == '\'' {
			inQuote = !inQuote
		}
		if !inQuote {
			r = toLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func toLower(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + 'a' - 'A'
	}
	return r
}

// unwrapOuterParens removes one pair of parentheses around the whole of s.
func unwrapOuterParens(s string) string {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return s
	}
	if closing, ok := topLevelScan(s); ok && closing == len(s)-1 {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// stripTrailingCast removes a final "::type" that applies to everything
// before it.
func stripTrailingCast(s string) string {
	i := lastTopLevelCast(s)
	if i <= 0 || !castTypeName.MatchString(strings.TrimSpace(s[i+2:])) {
		return s
	}
	return strings.TrimSpace(s[:i])
}

// topLevelScan returns the index of the parenthesis that closes s[0],
// ignoring parentheses inside string literals.
func topLevelScan(s string) (int, bool) {
	depth, inQuote := 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			inQuote = !inQuote
		case inQuote:
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// lastTopLevelCast returns the index of the last "::" outside parentheses
// and string literals, or -1.
func lastTopLevelCast(s string) int {
	last, depth, inQuote := -1, 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			inQuote = !inQuote
		case inQuote:
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ':' && depth == 0 && i+1 < len(s) && s[i+1] == ':':
			last = i
			i++
		}
	}
	return last
}
