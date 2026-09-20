// Package agentskill loads, validates and renders instructions for Go
// agents over Open Responses: the Agent Skills format, read through
// fs.FS so a skill can live in a directory, an embedded bundle, an
// archive or an adapter over a remote store, and the skill tool through
// which the model reads a skill's body and files.
//
// A [Skill] is one skill directory, loaded byte for byte: the parsed
// frontmatter, the Markdown body and the tree of resource files. [Load]
// reads one from any fs.FS and [Validate] reports the specification's
// rules with the same verdicts as the reference skills-ref validator.
// [Discover] loads every skill under an ordered list of [Source] values
// into a [Catalog], whose [Catalog.Prompt] renders the available_skills
// block a product puts in its instructions and whose [Catalog.Tool]
// serves the body and the files to the model.
//
// The package depends on openresponses, agenttool, one YAML parser and
// the standard library. The agent loop is never imported; a product
// wires the rendered prompt and the tool into its configuration. The
// nested instructions package handles the AGENTS.md convention and
// imports the standard library alone.
package agentskill

import (
	"io/fs"
	"strconv"
)

// Source is where skills come from: a file system and the name the
// prompt shows for it.
type Source struct {
	// FS holds skill directories as its direct children, each with a
	// SKILL.md. os.DirFS, embed.FS, a zip, or an adapter over a remote
	// store.
	FS fs.FS
	// Location names the source for the model, such as an absolute
	// directory, a URL or "mcp://docs". A skill's location is this
	// joined with the skill's directory name and SKILL.md.
	Location string
}

// Skill is one skill directory, loaded. The frontmatter fields carry
// the specification's keys; Name and Description are trimmed of
// surrounding space as the reference reader trims them, every other
// value is kept as written.
type Skill struct {
	// FS is the skill's own tree, rooted at its directory: a fs.Sub of
	// the source. SKILL.md is at its root. It is nil for a skill built
	// by [Parse].
	FS fs.FS
	// Location is what the prompt shows: the source location joined
	// with the directory name and SKILL.md.
	Location string
	// DirName is the directory's base name, which Name must match. It
	// is empty when the skill was not loaded from a directory, and the
	// match is then not checked.
	DirName string

	// Name is the frontmatter name.
	Name string
	// Description is the frontmatter description.
	Description string
	// License is the frontmatter license, if any.
	License string
	// Compatibility is the frontmatter compatibility, if any.
	Compatibility string
	// Metadata is the frontmatter metadata map, if any.
	Metadata map[string]string
	// AllowedTools is the frontmatter allowed-tools string, if any.
	// [Skill.Rules] parses it.
	AllowedTools string
	// Extra holds frontmatter keys the specification does not define,
	// decoded but not interpreted, so a newer skill loads in an older
	// reader and re-encodes without loss. Each is a validation error,
	// as the reference validator rejects them.
	Extra map[string]any

	// Body is the Markdown after the frontmatter, verbatim.
	Body string

	// keys records which frontmatter keys were present, so validation
	// can tell a missing name from an empty one. It is nil for a Skill
	// built as a literal, where a non-empty field counts as present.
	keys map[string]bool
	// shape holds problems found while decoding the frontmatter that
	// are about a value's shape rather than the YAML: a name that is a
	// list, a metadata value that is a mapping. They load and are
	// reported by Validate, so a product can list the skill and say
	// what is wrong with it.
	shape []Problem
}

// Severity says whether a [Problem] fails validation.
type Severity int

const (
	// Error is a rule of the specification the skill breaks. Any Error
	// problem fails validation.
	Error Severity = iota
	// Warning is advice the specification gives that the skill does
	// not follow. Warnings never fail validation.
	Warning
)

// String returns "error" or "warning".
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return "severity(" + strconv.Itoa(int(s)) + ")"
}

// Problem is one thing [Skill.Validate] found.
type Problem struct {
	Severity Severity
	// Field is the frontmatter key the problem is about, "body" for
	// the Markdown, or "" for the skill as a whole.
	Field string
	// Message is the reference validator's wording where it has one.
	Message string
}

// String returns the severity and the message, "error: ...".
func (p Problem) String() string { return p.Severity.String() + ": " + p.Message }

// HasErrors reports whether any problem is an [Error].
func HasErrors(problems []Problem) bool {
	for _, p := range problems {
		if p.Severity == Error {
			return true
		}
	}
	return false
}

// present reports whether the frontmatter had key. A literal Skill has
// no key record, so a non-empty field counts.
func (s *Skill) present(key string, value string) bool {
	if s.keys == nil {
		return value != ""
	}
	return s.keys[key]
}
