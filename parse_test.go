package agentskill

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want *Skill
		err  error
	}{
		{
			name: "minimal",
			src:  "---\nname: a\ndescription: b\n---\nBody\n",
			want: &Skill{Name: "a", Description: "b", Body: "Body\n"},
		},
		{
			name: "body verbatim with leading blank line",
			src:  "---\nname: a\ndescription: b\n---\n\n# Title\n",
			want: &Skill{Name: "a", Description: "b", Body: "\n# Title\n"},
		},
		{
			name: "no body",
			src:  "---\nname: a\ndescription: b\n---",
			want: &Skill{Name: "a", Description: "b"},
		},
		{
			name: "crlf",
			src:  "---\r\nname: a\r\ndescription: b\r\n---\r\nBody\r\n",
			want: &Skill{Name: "a", Description: "b", Body: "Body\r\n"},
		},
		{
			name: "every field",
			src:  "---\nname: a\ndescription: b\nlicense: MIT\ncompatibility: c\nallowed-tools: Bash(git:*) Read\nmetadata:\n  k: v\n  n: 1\n---\nBody",
			want: &Skill{Name: "a", Description: "b", License: "MIT", Compatibility: "c", AllowedTools: "Bash(git:*) Read",
				Metadata: map[string]string{"k": "v", "n": "1"}, Body: "Body"},
		},
		{
			name: "name and description trimmed, number as text",
			src:  "---\nname: '  a  '\ndescription: \"  b \"\nlicense: 1\n---\n",
			want: &Skill{Name: "a", Description: "b", License: "1"},
		},
		{
			name: "folded and literal scalars",
			src:  "---\nname: a\ndescription: >\n  one\n  two\nlicense: |\n  x\n  y\n---\n",
			want: &Skill{Name: "a", Description: "one two", License: "x\ny\n"},
		},
		{
			name: "extra keys kept",
			src:  "---\nname: a\ndescription: b\nversion: 2\ntags:\n  - x\n---\n",
			want: &Skill{Name: "a", Description: "b", Extra: map[string]any{"version": 2, "tags": []any{"x"}}},
		},
		{
			name: "dashes inside values are not fences",
			src:  "---\nname: a\ndescription: b --- c\n---\n--- not a fence\n",
			want: &Skill{Name: "a", Description: "b --- c", Body: "--- not a fence\n"},
		},
		{
			name: "empty frontmatter",
			src:  "---\n---\nBody",
			want: &Skill{Body: "Body"},
		},
		{name: "empty file", src: "", err: ErrNoFrontmatter},
		{name: "no frontmatter", src: "# Title\n", err: ErrNoFrontmatter},
		{name: "fence only", src: "---", err: ErrUnclosedFrontmatter},
		{name: "unclosed", src: "---\nname: a\n", err: ErrUnclosedFrontmatter},
		{name: "not a mapping", src: "---\n- a\n---\n", err: ErrNotMapping},
		{name: "invalid yaml", src: "---\nname: \"x\n---\n", err: errInvalidYAML},
		// A repeated key is a YAML error, as strictyaml makes it for
		// the reference reader: there is no correct value to load.
		{name: "duplicate key", src: "---\nname: a\ndescription: b\ndescription: c\n---\n", err: errInvalidYAML},
		{name: "duplicate unknown key", src: "---\nname: a\ndescription: b\nversion: 1\nversion: 2\n---\n", err: errInvalidYAML},
		{name: "duplicate name", src: "---\nname: a\nname: a\ndescription: b\n---\n", err: errInvalidYAML},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.src))
			if tt.err != nil {
				switch {
				case err == nil:
					t.Fatalf("Parse() = %+v, want error %v", got, tt.err)
				case tt.err == errInvalidYAML:
					if !strings.HasPrefix(err.Error(), "Invalid YAML in frontmatter: ") {
						t.Fatalf("Parse() error = %q, want an invalid YAML error", err)
					}
				case !errors.Is(err, tt.err):
					t.Fatalf("Parse() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got.keys, got.shape = nil, nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

// errInvalidYAML marks a test case whose error text comes from the
// YAML parser and is matched by prefix.
var errInvalidYAML = errors.New("invalid yaml")

func TestParseShapeProblems(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []Problem
	}{
		{
			name: "name is a list",
			src:  "---\nname:\n  - a\ndescription: b\n---\n",
			want: []Problem{{Severity: Error, Field: "name", Message: "Field 'name' must be a string"}},
		},
		{
			name: "metadata is a scalar",
			src:  "---\nname: a\ndescription: b\nmetadata: x\n---\n",
			want: []Problem{{Severity: Error, Field: "metadata", Message: "Field 'metadata' must be a mapping of strings to strings"}},
		},
		{
			name: "metadata value is a mapping",
			src:  "---\nname: a\ndescription: b\nmetadata:\n  ok: v\n  bad:\n    x: y\n---\n",
			want: []Problem{{Severity: Error, Field: "metadata", Message: "Field 'metadata' value for 'bad' must be a string"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("Parse() error = %v; shape problems must load", err)
			}
			if !reflect.DeepEqual(s.shape, tt.want) {
				t.Errorf("shape = %v, want %v", s.shape, tt.want)
			}
			got := s.Validate()
			for _, want := range tt.want {
				if !containsProblem(got, want) {
					t.Errorf("Validate() = %v, missing %v", got, want)
				}
			}
		})
	}
}

func containsProblem(problems []Problem, want Problem) bool {
	for _, p := range problems {
		if p == want {
			return true
		}
	}
	return false
}

func TestEncodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		s    *Skill
	}{
		{name: "minimal", s: &Skill{Name: "a", Description: "b", Body: "Body\n"}},
		{name: "no body", s: &Skill{Name: "a", Description: "b"}},
		{name: "every field", s: &Skill{Name: "a", Description: "b: with colon", License: "MIT", Compatibility: "c",
			AllowedTools: "Bash(git:*) Read", Metadata: map[string]string{"k": "v", "n": "1"}, Body: "\n# Title\n\ntext\n"}},
		{name: "multiline description", s: &Skill{Name: "a", Description: "one\ntwo", Body: "x"}},
		{name: "extra", s: &Skill{Name: "a", Description: "b", Extra: map[string]any{"version": 2, "tags": []any{"x", "y"}, "nested": map[string]any{"k": "v"}}}},
		{name: "body with fences", s: &Skill{Name: "a", Description: "b", Body: "---\nnot frontmatter\n---\n"}},
		{name: "quoting needed", s: &Skill{Name: "a", Description: "'quoted' \"and\" #hash", License: "yes", Body: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Encode(tt.s)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			got, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse(Encode()) error = %v\n%s", err, data)
			}
			got.keys, got.shape = nil, nil
			if !reflect.DeepEqual(got, tt.s) {
				t.Errorf("Parse(Encode()) =\n%#v\nwant\n%#v\nencoded:\n%s", got, tt.s, data)
			}
		})
	}
}

func TestEncodeOrder(t *testing.T) {
	s := &Skill{Name: "a", Description: "b", License: "MIT", Compatibility: "c", AllowedTools: "Read",
		Metadata: map[string]string{"z": "1", "a": "2"}, Extra: map[string]any{"zeta": 1, "alpha": 2}, Body: "Body\n"}
	got, err := Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nname: a\ndescription: b\nlicense: MIT\ncompatibility: c\nallowed-tools: Read\nmetadata:\n  a: \"2\"\n  z: \"1\"\nalpha: 2\nzeta: 1\n---\nBody\n"
	if string(got) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", got, want)
	}
}
