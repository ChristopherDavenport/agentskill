package agentskill

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		s    Skill
		want []Problem
	}{
		{name: "literal valid", s: Skill{Name: "ok-name", Description: "d"}},
		{name: "unicode letters allowed", s: Skill{Name: "café-ünïcode-日本", Description: "d"}},
		{name: "digits allowed", s: Skill{Name: "v2-tools", Description: "d"}},
		{name: "directory matches", s: Skill{Name: "a", Description: "d", DirName: "a"}},
		{
			name: "literal missing fields",
			s:    Skill{},
			want: []Problem{
				{Severity: Error, Field: "name", Message: "Missing required field in frontmatter: name"},
				{Severity: Error, Field: "description", Message: "Missing required field in frontmatter: description"},
			},
		},
		{
			name: "directory mismatch",
			s:    Skill{Name: "a", Description: "d", DirName: "b"},
			want: []Problem{{Severity: Error, Field: "name", Message: "Directory name 'b' must match skill name 'a'"}},
		},
		{
			name: "uppercase unicode",
			s:    Skill{Name: "Émile", Description: "d"},
			want: []Problem{{Severity: Error, Field: "name", Message: "Skill name 'Émile' must be lowercase"}},
		},
		{
			name: "length in characters",
			s:    Skill{Name: strings.Repeat("é", 64), Description: strings.Repeat("é", 1024), Compatibility: strings.Repeat("é", 500)},
		},
		{
			name: "one over each limit",
			s:    Skill{Name: strings.Repeat("a", 65), Description: strings.Repeat("b", 1025), Compatibility: strings.Repeat("c", 501)},
			want: []Problem{
				{Severity: Error, Field: "name", Message: "Skill name '" + strings.Repeat("a", 65) + "' exceeds 64 character limit (65 chars)"},
				{Severity: Error, Field: "description", Message: "Description exceeds 1024 character limit (1025 chars)"},
				{Severity: Error, Field: "compatibility", Message: "Compatibility exceeds 500 character limit (501 chars)"},
			},
		},
		{
			name: "allowed-tools token",
			s:    Skill{Name: "a", Description: "d", AllowedTools: "Read Bash(git"},
			want: []Problem{{Severity: Error, Field: "allowed-tools", Message: `allowed-tools token "Bash(git": unmatched parenthesis`}},
		},
		{
			// The documented form of the field: specifiers hold spaces.
			name: "allowed-tools with spaces",
			s:    Skill{Name: "a", Description: "d", AllowedTools: "Bash(git add *) Bash(git commit *) Bash(git status *)"},
		},
		{
			name: "long body warns",
			s:    Skill{Name: "a", Description: "d", Body: strings.Repeat("line\n", 501)},
			want: []Problem{{Severity: Warning, Field: "body", Message: "SKILL.md has 501 lines; the specification advises keeping it under 500"}},
		},
		{name: "body at the limit", s: Skill{Name: "a", Description: "d", Body: strings.Repeat("line\n", 500)}},
		{
			name: "extra keys",
			s:    Skill{Name: "a", Description: "d", Extra: map[string]any{"z": 1, "b": 2}},
			want: []Problem{{Severity: Error, Message: "Unexpected fields in frontmatter: b, z. Only ['allowed-tools', 'compatibility', 'description', 'license', 'metadata', 'name'] are allowed."}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s.Validate()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Validate() =\n%v\nwant\n%v", got, tt.want)
			}
			if HasErrors(got) != hasSeverity(tt.want, Error) {
				t.Errorf("HasErrors() = %v", HasErrors(got))
			}
		})
	}
}

func hasSeverity(problems []Problem, sev Severity) bool {
	for _, p := range problems {
		if p.Severity == sev {
			return true
		}
	}
	return false
}

func TestRules(t *testing.T) {
	tests := []struct {
		in   string
		want []ToolRule
		err  string
	}{
		{in: ""},
		{in: "   "},
		{in: "Read", want: []ToolRule{{Tool: "Read"}}},
		{in: "Bash(git:*) Read\tWrite(*.md)", want: []ToolRule{{Tool: "Bash", Spec: "git:*"}, {Tool: "Read"}, {Tool: "Write", Spec: "*.md"}}},
		// A token that opens a specifier and supplies none is refused,
		// as agentpolicy refuses it; see
		// TestEmptySpecifierNeverBecomesARule.
		{in: "Bash()", err: `allowed-tools token "Bash()": empty specifier`},
		{in: "Bash(!)", err: `allowed-tools token "Bash(!)": empty carve-out`},
		{in: "Read Bash() Write", err: `allowed-tools token "Bash()": empty specifier`},
		{in: "Bash(())", want: []ToolRule{{Tool: "Bash", Spec: "()"}}},
		{in: "Bash(a(b))", want: []ToolRule{{Tool: "Bash", Spec: "a(b)"}}},
		// Whitespace inside parentheses does not end a token, so the
		// documented examples parse: "Bash(git add *)" and a path with
		// literal parentheses.
		{
			in:   "Bash(git add *) Bash(git commit *) Bash(git status *)",
			want: []ToolRule{{Tool: "Bash", Spec: "git add *"}, {Tool: "Bash", Spec: "git commit *"}, {Tool: "Bash", Spec: "git status *"}},
		},
		{in: "Edit(./Finance (2024)/**)  Read", want: []ToolRule{{Tool: "Edit", Spec: "./Finance (2024)/**"}, {Tool: "Read"}}},
		{in: "Bash( a  b )", want: []ToolRule{{Tool: "Bash", Spec: " a  b "}}},
		{in: "(x)", err: `allowed-tools token "(x)": missing tool name`},
		{in: "Bash(x", err: `allowed-tools token "Bash(x": unmatched parenthesis`},
		{in: "Bash(git add *", err: `allowed-tools token "Bash(git add *": unmatched parenthesis`},
		{in: "Bash)", err: `allowed-tools token "Bash)": unmatched parenthesis`},
		{in: "Read )", err: `allowed-tools token ")": unmatched parenthesis`},
		{in: "Bash(x)y", err: `allowed-tools token "Bash(x)y": text after the specifier`},
		{in: "Bash(x)(y)", err: `allowed-tools token "Bash(x)(y)": text after the specifier`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := (&Skill{AllowedTools: tt.in}).Rules()
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("Rules() error = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Rules() = %v, want %v", got, tt.want)
			}
			for _, r := range got {
				if !r.Matches(r.Tool) || r.Matches(r.Tool+"x") {
					t.Errorf("Matches on %v is wrong", r)
				}
			}
		})
	}
}

// TestEmptySpecifierNeverBecomesARule pins the seam this package
// shares with agentpolicy, where a ToolRule maps onto a Rule field for
// field: a Rule whose Spec is empty is bare, and a bare rule matches
// every call of the tool. So a token that names a specifier and
// supplies none must never leave Rules as a rule; it is a problem on
// the allowed-tools field, naming the token, and the skill does not
// validate.
func TestEmptySpecifierNeverBecomesARule(t *testing.T) {
	tests := []struct {
		name    string
		allowed string
		want    string
	}{
		{name: "alone", allowed: "Bash()", want: `allowed-tools token "Bash()": empty specifier`},
		{name: "beside a bare rule", allowed: "Bash() Read", want: `allowed-tools token "Bash()": empty specifier`},
		{name: "after a real specifier", allowed: "Bash(git status:*) Bash()", want: `allowed-tools token "Bash()": empty specifier`},
		{name: "empty carve-out", allowed: "Bash(!)", want: `allowed-tools token "Bash(!)": empty carve-out`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Skill{Name: "wide-open", Description: "d", AllowedTools: tt.allowed}
			rules, err := s.Rules()
			if err == nil {
				t.Fatalf("Rules() = %v, want an error", rules)
			}
			if err.Error() != tt.want {
				t.Errorf("Rules() error = %q, want %q", err, tt.want)
			}
			if rules != nil {
				t.Errorf("Rules() = %v, want no rules", rules)
			}
			problems := s.Validate()
			want := []Problem{{Severity: Error, Field: "allowed-tools", Message: tt.want}}
			if !reflect.DeepEqual(problems, want) {
				t.Errorf("Validate() = %v, want %v", problems, want)
			}
			if !HasErrors(problems) {
				t.Error("the skill validates clean")
			}
		})
	}
}

func TestToolRuleString(t *testing.T) {
	for _, in := range []string{"Read", "Bash(git:*)", "Bash(git add *)", "Edit(./Finance (2024)/**)"} {
		r, err := parseRule(in)
		if err != nil {
			t.Fatal(err)
		}
		if r.String() != in {
			t.Errorf("String() = %q, want %q", r.String(), in)
		}
	}
}
