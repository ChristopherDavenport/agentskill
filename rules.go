package agentskill

import (
	"fmt"
	"strings"
	"unicode"
)

// ToolRule is one token of allowed-tools: a tool name, or a name with
// a specifier in parentheses, "Bash(git:*)". The specifier's syntax
// belongs to the product; the rule keeps it as a string, spaces and
// nested parentheses included, as in "Bash(git add *)" and
// "Edit(./Finance (2024)/**)".
type ToolRule struct {
	Tool string
	// Spec is the text inside the parentheses, "" when the token has
	// no parentheses at all. A token whose parentheses are empty does
	// not parse; see [Skill.Rules].
	Spec string
}

// String returns the token as it was written.
func (r ToolRule) String() string {
	if r.Spec == "" {
		return r.Tool
	}
	return r.Tool + "(" + r.Spec + ")"
}

// Matches reports whether the rule names the tool. The specifier is
// not consulted; a host that interprets it checks Spec itself.
func (r ToolRule) Matches(toolName string) bool { return r.Tool == toolName }

// Rules parses AllowedTools into one [ToolRule] per token. Tokens are
// separated by whitespace outside parentheses, so a specifier may hold
// spaces; unbalanced parentheses are an error, because the token
// cannot then be delimited. An empty field yields no rules and no
// error.
//
// A token that opens a specifier and supplies none, "Bash()", or whose
// specifier is the bare carve-out "Bash(!)", is an error rather than a
// rule, as agentpolicy's parser of the same grammar refuses both. An
// empty specifier would otherwise be indistinguishable from the bare
// token "Bash", which grants every call of the tool, so the narrowest
// thing a skill can write would arrive as the widest grant there is.
func (s *Skill) Rules() ([]ToolRule, error) {
	tokens, err := splitRules(s.AllowedTools)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	rules := make([]ToolRule, 0, len(tokens))
	for _, tok := range tokens {
		r, err := parseRule(tok)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// splitRules cuts the field at whitespace outside parentheses. A
// closing parenthesis with no open one, or an open one never closed,
// is reported with the token it belongs to.
func splitRules(s string) ([]string, error) {
	var tokens []string
	depth, start := 0, -1
	for i, r := range s {
		if depth == 0 && unicode.IsSpace(r) {
			if start >= 0 {
				tokens = append(tokens, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, unmatched(s[start : i+1])
			}
			depth--
		}
	}
	if depth != 0 {
		return nil, unmatched(s[start:])
	}
	if start >= 0 {
		tokens = append(tokens, s[start:])
	}
	return tokens, nil
}

func unmatched(tok string) error {
	return fmt.Errorf("allowed-tools token %q: unmatched parenthesis", tok)
}

// parseRule reads one token: the tool name, then optionally a
// specifier that runs from the first open parenthesis to the one
// matching it, which must end the token.
func parseRule(tok string) (ToolRule, error) {
	open := strings.IndexByte(tok, '(')
	if open < 0 {
		if strings.ContainsRune(tok, ')') {
			return ToolRule{}, unmatched(tok)
		}
		return ToolRule{Tool: tok}, nil
	}
	if open == 0 {
		return ToolRule{}, fmt.Errorf("allowed-tools token %q: missing tool name", tok)
	}
	depth := 0
	for i := open; i < len(tok); i++ {
		switch tok[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if i != len(tok)-1 {
					return ToolRule{}, fmt.Errorf("allowed-tools token %q: text after the specifier", tok)
				}
				spec := tok[open+1 : i]
				switch spec {
				case "":
					return ToolRule{}, fmt.Errorf("allowed-tools token %q: empty specifier", tok)
				case "!":
					return ToolRule{}, fmt.Errorf("allowed-tools token %q: empty carve-out", tok)
				}
				return ToolRule{Tool: tok[:open], Spec: spec}, nil
			}
		}
	}
	return ToolRule{}, unmatched(tok)
}
