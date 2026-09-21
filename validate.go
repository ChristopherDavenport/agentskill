package agentskill

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The specification's limits.
const (
	MaxNameLength          = 64
	MaxDescriptionLength   = 1024
	MaxCompatibilityLength = 500
	// MaxBodyLines is the length the specification advises a SKILL.md
	// to stay under. A longer body is a warning.
	MaxBodyLines = 500
)

// Validate checks the skill against the specification and returns one
// [Problem] per rule broken, in the reference validator's order and
// with its wording: unknown frontmatter keys, then name, description
// and compatibility. Rules the reference validator does not check
// follow: metadata values that are not strings, allowed-tools tokens
// that do not parse, and a body over [MaxBodyLines] lines, which is a
// [Warning]. A nil result means the skill is valid.
//
// The verdicts and their wording are the reference validator's over
// the frontmatter both readers parse. Three classes of input fall
// outside that: Unicode normalisation, the YAML dialect, and an empty
// allowed-tools specifier, which [Skill.Rules] refuses and the
// reference accepts. The README names all three.
//
// Name length is counted in characters, not bytes. The directory match
// is a plain comparison; the reference validator normalises both sides
// to NFKC first, which the standard library cannot do, so a name and
// directory that differ only in Unicode normalisation pass there and
// fail here.
func (s *Skill) Validate() []Problem {
	var problems []Problem
	if len(s.Extra) > 0 {
		problems = append(problems, Problem{Severity: Error, Message: fmt.Sprintf(
			"Unexpected fields in frontmatter: %s. Only %s are allowed.",
			strings.Join(sortedKeys(s.Extra), ", "), pythonList(knownKeys))})
	}
	problems = append(problems, s.shape...)
	switch {
	case !s.present(keyName, s.Name):
		problems = append(problems, Problem{Severity: Error, Field: keyName, Message: "Missing required field in frontmatter: name"})
	case !s.hasShapeProblem(keyName):
		problems = append(problems, s.validateName()...)
	}
	switch {
	case !s.present(keyDescription, s.Description):
		problems = append(problems, Problem{Severity: Error, Field: keyDescription, Message: "Missing required field in frontmatter: description"})
	case !s.hasShapeProblem(keyDescription):
		problems = append(problems, s.validateDescription()...)
	}
	if s.present(keyCompatibility, s.Compatibility) && !s.hasShapeProblem(keyCompatibility) {
		if n := utf8.RuneCountInString(s.Compatibility); n > MaxCompatibilityLength {
			problems = append(problems, Problem{Severity: Error, Field: keyCompatibility, Message: fmt.Sprintf(
				"Compatibility exceeds %d character limit (%d chars)", MaxCompatibilityLength, n)})
		}
	}
	if s.AllowedTools != "" {
		if _, err := s.Rules(); err != nil {
			problems = append(problems, Problem{Severity: Error, Field: keyAllowedTools, Message: err.Error()})
		}
	}
	if n := countLines(s.Body); n > MaxBodyLines {
		problems = append(problems, Problem{Severity: Warning, Field: "body", Message: fmt.Sprintf(
			"SKILL.md has %d lines; the specification advises keeping it under %d", n, MaxBodyLines)})
	}
	return problems
}

func (s *Skill) hasShapeProblem(field string) bool {
	for _, p := range s.shape {
		if p.Field == field {
			return true
		}
	}
	return false
}

func (s *Skill) validateName() []Problem {
	name := s.Name
	if name == "" {
		return []Problem{{Severity: Error, Field: keyName, Message: "Field 'name' must be a non-empty string"}}
	}
	var problems []Problem
	add := func(msg string) {
		problems = append(problems, Problem{Severity: Error, Field: keyName, Message: msg})
	}
	if n := utf8.RuneCountInString(name); n > MaxNameLength {
		add(fmt.Sprintf("Skill name '%s' exceeds %d character limit (%d chars)", name, MaxNameLength, n))
	}
	if name != strings.ToLower(name) {
		add(fmt.Sprintf("Skill name '%s' must be lowercase", name))
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		add("Skill name cannot start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		add("Skill name cannot contain consecutive hyphens")
	}
	if strings.ContainsFunc(name, func(r rune) bool { return r != '-' && !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		add(fmt.Sprintf("Skill name '%s' contains invalid characters. Only letters, digits, and hyphens are allowed.", name))
	}
	if s.DirName != "" && s.DirName != name {
		add(fmt.Sprintf("Directory name '%s' must match skill name '%s'", s.DirName, name))
	}
	return problems
}

func (s *Skill) validateDescription() []Problem {
	if s.Description == "" {
		return []Problem{{Severity: Error, Field: keyDescription, Message: "Field 'description' must be a non-empty string"}}
	}
	if n := utf8.RuneCountInString(s.Description); n > MaxDescriptionLength {
		return []Problem{{Severity: Error, Field: keyDescription, Message: fmt.Sprintf(
			"Description exceeds %d character limit (%d chars)", MaxDescriptionLength, n)}}
	}
	return nil
}

// countLines counts the lines of a body the way an editor does: a
// trailing newline does not start another line.
func countLines(body string) int {
	if body == "" {
		return 0
	}
	n := strings.Count(body, "\n")
	if !strings.HasSuffix(body, "\n") {
		n++
	}
	return n
}

// pythonList renders keys as Python's repr of a sorted list, which is
// how the reference validator names the allowed fields in its message.
func pythonList(keys []string) string {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	quoted := make([]string, len(sorted))
	for i, k := range sorted {
		quoted[i] = "'" + k + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
