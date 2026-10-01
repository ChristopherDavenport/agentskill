package agentskill

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// grammarErrors maps agentpolicy's error kinds onto the message this
// parser gives for the same refusal.
var grammarErrors = map[string]string{
	"unbalanced_parentheses": "unmatched parenthesis",
	"missing_tool_name":      "missing tool name",
	"empty_specifier":        "empty specifier",
	"empty_carve_out":        "empty carve-out",
	"text_after_specifier":   "text after the specifier",
}

// TestGrammarConformance runs agentpolicy's grammar cases against
// Rules. testdata/policy/grammar.json is a verbatim copy of
// agentpolicy v0.0.9's testdata/policy/grammar.json, whose RFC 0001
// names this parser as a conforming one: for every case it must give
// the rules the case lists, or refuse with the kind and token it names.
// Recopy the file when agentpolicy changes it.
func TestGrammarConformance(t *testing.T) {
	data, err := os.ReadFile("testdata/policy/grammar.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name  string `json:"name"`
			Input string `json:"input"`
			Rules []struct {
				Tool string `json:"tool"`
				Spec string `json:"spec"`
			} `json:"rules"`
			Error *struct {
				Kind  string `json:"kind"`
				Token string `json:"token"`
			} `json:"error"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("grammar.json has no cases")
	}
	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := (&Skill{AllowedTools: c.Input}).Rules()
			if c.Error != nil {
				msg, ok := grammarErrors[c.Error.Kind]
				if !ok {
					t.Fatalf("unknown error kind %q", c.Error.Kind)
				}
				want := fmt.Sprintf("allowed-tools token %q: %s", c.Error.Token, msg)
				if err == nil || err.Error() != want {
					t.Fatalf("Rules(%q) = %v, %v; want error %q", c.Input, got, err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Rules(%q) error = %v", c.Input, err)
			}
			want := make([]ToolRule, 0, len(c.Rules))
			for _, r := range c.Rules {
				want = append(want, ToolRule{Tool: r.Tool, Spec: r.Spec})
			}
			if len(got) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Rules(%q) = %#v, want %#v", c.Input, got, want)
			}
		})
	}
}
