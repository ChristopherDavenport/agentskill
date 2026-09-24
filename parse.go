package agentskill

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The frontmatter keys the specification defines, in the order Encode
// writes them.
const (
	keyName          = "name"
	keyDescription   = "description"
	keyLicense       = "license"
	keyCompatibility = "compatibility"
	keyAllowedTools  = "allowed-tools"
	keyMetadata      = "metadata"
)

var knownKeys = []string{keyName, keyDescription, keyLicense, keyCompatibility, keyAllowedTools, keyMetadata}

// Errors from [Parse]. They are about the frontmatter fence and the
// YAML alone; a value of the wrong shape loads and is reported by
// [Skill.Validate].
var (
	// ErrNoFrontmatter is returned when the file does not begin with
	// the opening fence.
	ErrNoFrontmatter = errors.New("SKILL.md must start with YAML frontmatter (---)")
	// ErrUnclosedFrontmatter is returned when the closing fence is
	// missing.
	ErrUnclosedFrontmatter = errors.New("SKILL.md frontmatter not properly closed with ---")
	// ErrNotMapping is returned when the frontmatter is valid YAML but
	// not a mapping.
	ErrNotMapping = errors.New("SKILL.md frontmatter must be a YAML mapping")
)

// Parse reads the frontmatter and the body of a SKILL.md from src. The
// result has no FS, Location or DirName; [Load] fills those in. Parse
// fails only on the fence and the YAML: a value of the wrong shape,
// such as a name that is a list, loads with an empty field and is
// reported by [Skill.Validate].
//
// A key written twice is one of the YAML failures, as it is for the
// reference reader's strictyaml, rather than a problem to report: the
// file says two things with equal authority and there is no correct
// value to load. A reviewer's eye stops at the first line, and the
// model would be told the second.
//
// The fence is line based: the first line must be "---" and the
// frontmatter runs to the next line that is "---". The body is
// everything after that line, verbatim. Scalars are read as their
// text, so a name written as a number is the number's text, as the
// reference reader does.
func Parse(src []byte) (*Skill, error) {
	front, body, err := splitFrontmatter(src)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		//lint:ignore ST1005 mirrors the reference validator's wording
		return nil, fmt.Errorf("Invalid YAML in frontmatter: %w", err)
	}
	s := &Skill{Body: body, keys: map[string]bool{}}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		// Empty frontmatter: a mapping with no keys, so validation
		// reports the missing fields.
		return s, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, ErrNotMapping
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			//lint:ignore ST1005 mirrors the reference validator's wording
			return nil, fmt.Errorf("Invalid YAML in frontmatter: line %d: mapping key is not a scalar", k.Line)
		}
		key := k.Value
		if s.keys[key] {
			//lint:ignore ST1005 mirrors the reference validator's wording
			return nil, fmt.Errorf("Invalid YAML in frontmatter: line %d: duplicate key %q", k.Line, key)
		}
		s.keys[key] = true
		switch key {
		case keyName:
			s.Name = strings.TrimSpace(s.scalar(key, v))
		case keyDescription:
			s.Description = strings.TrimSpace(s.scalar(key, v))
		case keyLicense:
			s.License = s.scalar(key, v)
		case keyCompatibility:
			s.Compatibility = s.scalar(key, v)
		case keyAllowedTools:
			s.AllowedTools = s.scalar(key, v)
		case keyMetadata:
			s.Metadata = s.metadata(v)
		default:
			var extra any
			if err := v.Decode(&extra); err != nil {
				//lint:ignore ST1005 mirrors the reference validator's wording
				return nil, fmt.Errorf("Invalid YAML in frontmatter: %w", err)
			}
			if s.Extra == nil {
				s.Extra = map[string]any{}
			}
			s.Extra[key] = extra
		}
	}
	return s, nil
}

// scalar returns the text of a scalar node, or records a shape problem
// and returns "" for anything else.
func (s *Skill) scalar(key string, v *yaml.Node) string {
	if v.Kind == yaml.ScalarNode {
		return v.Value
	}
	s.shape = append(s.shape, Problem{Severity: Error, Field: key, Message: fmt.Sprintf("Field '%s' must be a string", key)})
	return ""
}

// metadata decodes the metadata mapping. Values must be scalars; a
// mapping or a list value is a shape problem, since the specification
// says string to string.
func (s *Skill) metadata(v *yaml.Node) map[string]string {
	if v.Kind != yaml.MappingNode {
		s.shape = append(s.shape, Problem{Severity: Error, Field: keyMetadata, Message: "Field 'metadata' must be a mapping of strings to strings"})
		return nil
	}
	out := make(map[string]string, len(v.Content)/2)
	for i := 0; i+1 < len(v.Content); i += 2 {
		k, val := v.Content[i], v.Content[i+1]
		if k.Kind != yaml.ScalarNode || val.Kind != yaml.ScalarNode {
			s.shape = append(s.shape, Problem{Severity: Error, Field: keyMetadata, Message: fmt.Sprintf("Field 'metadata' value for '%s' must be a string", k.Value)})
			continue
		}
		out[k.Value] = val.Value
	}
	return out
}

// splitFrontmatter separates the fenced YAML from the body.
func splitFrontmatter(src []byte) (front []byte, body string, err error) {
	first, rest, hasNewline := bytes.Cut(src, []byte("\n"))
	if !isFence(first) {
		return nil, "", ErrNoFrontmatter
	}
	if !hasNewline {
		return nil, "", ErrUnclosedFrontmatter
	}
	offset := 0
	for offset <= len(rest) {
		line, after, more := bytes.Cut(rest[offset:], []byte("\n"))
		if isFence(line) {
			return rest[:offset], string(after), nil
		}
		if !more {
			break
		}
		offset += len(line) + 1
	}
	return nil, "", ErrUnclosedFrontmatter
}

// isFence reports whether a line is the "---" fence, with an optional
// carriage return for files with Windows line endings.
func isFence(line []byte) bool {
	line = bytes.TrimSuffix(line, []byte("\r"))
	return string(line) == "---"
}

// Encode writes the skill back as a SKILL.md: the frontmatter with the
// specification's keys in specification order, Extra keys after them
// in sorted order, then the body verbatim. Parse(Encode(s)) yields s.
// Optional fields that are empty are omitted, as is a value that
// failed to decode into its field, since the Skill never held it.
func Encode(s *Skill) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(key string, value *yaml.Node) {
		root.Content = append(root.Content, scalarNode(key), value)
	}
	add(keyName, scalarNode(s.Name))
	add(keyDescription, scalarNode(s.Description))
	if s.License != "" {
		add(keyLicense, scalarNode(s.License))
	}
	if s.Compatibility != "" {
		add(keyCompatibility, scalarNode(s.Compatibility))
	}
	if s.AllowedTools != "" {
		add(keyAllowedTools, scalarNode(s.AllowedTools))
	}
	if len(s.Metadata) > 0 {
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range sortedKeys(s.Metadata) {
			m.Content = append(m.Content, scalarNode(k), scalarNode(s.Metadata[k]))
		}
		add(keyMetadata, m)
	}
	for _, k := range sortedKeys(s.Extra) {
		var v yaml.Node
		if err := v.Encode(s.Extra[k]); err != nil {
			return nil, fmt.Errorf("agentskill: encode %q: %w", k, err)
		}
		add(k, &v)
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("agentskill: encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("agentskill: encode frontmatter: %w", err)
	}
	out.WriteString("---\n")
	out.WriteString(s.Body)
	return out.Bytes(), nil
}

// scalarNode builds a string scalar, quoted only when the encoder must.
func scalarNode(v string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	if strings.Contains(v, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}

// sortedKeys returns the keys of m in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
