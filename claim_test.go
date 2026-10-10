package agentskill

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ChristopherDavenport/agenttool"
)

// readClaim is the claim fn of the tests: a read of the file, under
// the skill's listed name, as a product holding the skill tool to its
// read tool's rules would make it.
func readClaim(_ context.Context, s *Skill, file string) ([]agenttool.FactCall, error) {
	args, err := json.Marshal(map[string]string{"skill": s.ListedName(), "path": file})
	if err != nil {
		return nil, err
	}
	return []agenttool.FactCall{{Tool: "read", Args: args, Text: s.ListedName() + "/" + file}}, nil
}

// The claim names exactly the file the call serves: the skill file as
// loaded for the instructions, the path for a file, and nothing beyond
// the call itself for a call the tool refuses before reading anything.
func TestFileClaimNamesTheFileServed(t *testing.T) {
	tool := fixtureCatalog(t).Tool(WithFileClaim(readClaim))
	tests := []struct {
		name string
		args string
		// claim is the "skill/file" fn is asked about, "" when fn must
		// not be asked and the claim is the call itself alone.
		claim string
		// served says Execute serves the file claimed; otherwise it
		// refuses the call.
		served bool
	}{
		{name: "instructions", args: `{"name":"pdf-processing"}`, claim: "pdf-processing/SKILL.md", served: true},
		{name: "instructions by an empty path", args: `{"name":"pdf-processing","path":""}`, claim: "pdf-processing/SKILL.md", served: true},
		{name: "instructions as SKILL.md", args: `{"name":"pdf-processing","path":"SKILL.md"}`, claim: "pdf-processing/SKILL.md", served: true},
		{name: "instructions as skill.MD", args: `{"name":"pdf-processing","path":"skill.MD"}`, claim: "pdf-processing/SKILL.md", served: true},
		// The skill file as loaded, not as the call spells it.
		{name: "instructions of a lowercase skill file", args: `{"name":"lowercase-skill-md","path":"SKILL.md"}`, claim: "lowercase-skill-md/skill.md", served: true},
		{name: "resource file", args: `{"name":"pdf-processing","path":"reference.md"}`, claim: "pdf-processing/reference.md", served: true},
		{name: "nested file", args: `{"name":"pdf-processing","path":"scripts/extract.py"}`, claim: "pdf-processing/scripts/extract.py", served: true},
		{name: "image", args: `{"name":"pdf-processing","path":"assets/logo.png"}`, claim: "pdf-processing/assets/logo.png", served: true},
		// Read before it is refused, so claimed.
		{name: "binary file", args: `{"name":"pdf-processing","path":"assets/blob.bin"}`, claim: "pdf-processing/assets/blob.bin"},
		{name: "a nested SKILL.md is a file", args: `{"name":"pdf-processing","path":"scripts/SKILL.md"}`, claim: "pdf-processing/scripts/SKILL.md"},
		{name: "missing file", args: `{"name":"pdf-processing","path":"missing.md"}`, claim: "pdf-processing/missing.md"},
		{name: "parent", args: `{"name":"pdf-processing","path":".."}`},
		{name: "traversal", args: `{"name":"pdf-processing","path":"../minimal/SKILL.md"}`},
		{name: "absolute", args: `{"name":"pdf-processing","path":"/etc/passwd"}`},
		{name: "dot slash", args: `{"name":"pdf-processing","path":"./reference.md"}`},
		{name: "unknown skill", args: `{"name":"nope","path":"reference.md"}`},
		{name: "missing name", args: `{"path":"reference.md"}`},
		{name: "arguments that do not decode", args: `{"name":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := json.RawMessage(tt.args)
			facts, factual, err := agenttool.FactsOf(context.Background(), tool, args)
			if err != nil || !factual {
				t.Fatalf("FactsOf() = %v, %v, want a claim", factual, err)
			}
			if len(facts.Calls) == 0 || facts.Calls[0].Tool != "" || string(facts.Calls[0].Args) != tt.args {
				t.Fatalf("claim = %+v, want the call itself first", facts.Calls)
			}
			if facts.Rewrite != nil {
				t.Errorf("Rewrite = %s, want none", facts.Rewrite)
			}
			var claimed []string
			for _, c := range facts.Calls[1:] {
				claimed = append(claimed, c.Text)
			}
			var want []string
			if tt.claim != "" {
				want = []string{tt.claim}
			}
			if !reflect.DeepEqual(claimed, want) {
				t.Fatalf("claimed %q, want %q", claimed, want)
			}

			res, err := tool.Execute(context.Background(), agenttool.Call{ID: "c1", Args: args})
			if !tt.served {
				if err == nil {
					t.Fatalf("Execute served %+v, want a refusal", res.Details)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			read := res.Details.(Read)
			got := read.Name + "/" + read.Path
			if read.Path == "" {
				// The instructions: the skill file behind Location.
				got = read.Name + "/" + read.Location[strings.LastIndex(read.Location, "/")+1:]
			}
			if got != tt.claim {
				t.Errorf("served %q, claimed %q", got, tt.claim)
			}
		})
	}
}

func TestFileClaimError(t *testing.T) {
	boom := errors.New("boom")
	tool := fixtureCatalog(t).Tool(WithFileClaim(func(context.Context, *Skill, string) ([]agenttool.FactCall, error) {
		return nil, boom
	}))
	for _, args := range []string{`{"name":"pdf-processing"}`, `{"name":"pdf-processing","path":"reference.md"}`} {
		f, factual, err := agenttool.FactsOf(context.Background(), tool, json.RawMessage(args))
		if !factual || !errors.Is(err, boom) {
			t.Errorf("%s: FactsOf() = %+v, %v, %v, want fn's error", args, f, factual, err)
		}
	}
	// A call refused before fn is asked has no error to report.
	if _, _, err := agenttool.FactsOf(context.Background(), tool, json.RawMessage(`{"name":"nope"}`)); err != nil {
		t.Errorf("unknown skill: %v, want the call itself", err)
	}
}

// fn's answer may be empty, and the claim is then the call itself.
func TestFileClaimOfNothing(t *testing.T) {
	tool := fixtureCatalog(t).Tool(WithFileClaim(func(context.Context, *Skill, string) ([]agenttool.FactCall, error) {
		return nil, nil
	}))
	args := json.RawMessage(`{"name":"minimal"}`)
	f, _, err := agenttool.FactsOf(context.Background(), tool, args)
	if err != nil {
		t.Fatal(err)
	}
	if want := []agenttool.FactCall{{Args: args}}; !reflect.DeepEqual(f.Calls, want) {
		t.Errorf("claim = %+v, want %+v", f.Calls, want)
	}
}

// A skill Load did not build serves the body it holds and reads no
// file, so its instructions claim the call itself alone.
func TestFileClaimOfAParsedSkill(t *testing.T) {
	s, err := Parse([]byte("---\nname: a\ndescription: d\n---\nBody."))
	if err != nil {
		t.Fatal(err)
	}
	asked := false
	tool := (&Catalog{Skills: []*Skill{s}}).Tool(WithFileClaim(func(context.Context, *Skill, string) ([]agenttool.FactCall, error) {
		asked = true
		return nil, nil
	}))
	args := json.RawMessage(`{"name":"a"}`)
	f, _, err := agenttool.FactsOf(context.Background(), tool, args)
	if err != nil {
		t.Fatal(err)
	}
	if asked || len(f.Calls) != 1 {
		t.Errorf("claim = %+v, asked = %v; want the call itself alone", f.Calls, asked)
	}
	if _, err := call(t, tool, string(args)); err != nil {
		t.Errorf("Execute: %v", err)
	}
}

func TestFileClaimIsOptional(t *testing.T) {
	for name, tool := range map[string]agenttool.Tool{
		"no option": fixtureCatalog(t).Tool(),
		"nil fn":    fixtureCatalog(t).Tool(WithFileClaim(nil)),
	} {
		if agenttool.IsFactual(tool) {
			t.Errorf("%s: the tool makes the facts claim", name)
		}
		if _, factual, _ := agenttool.FactsOf(context.Background(), tool, json.RawMessage(`{"name":"minimal"}`)); factual {
			t.Errorf("%s: FactsOf reports a claim", name)
		}
	}
}

// A product that wraps the skill tool, as agentkit does to grant on
// use, keeps the claim.
func TestFileClaimSurvivesWrap(t *testing.T) {
	tool := fixtureCatalog(t).Tool(WithFileClaim(readClaim))
	wrapped := agenttool.Wrap(tool, tool.Execute)
	if !agenttool.IsFactual(wrapped) {
		t.Fatal("the wrapped tool makes no claim")
	}
	args := json.RawMessage(`{"name":"pdf-processing","path":"scripts/extract.py"}`)
	want, _, err := agenttool.FactsOf(context.Background(), tool, args)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := agenttool.FactsOf(context.Background(), wrapped, args)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || len(got.Calls) != 2 || got.Calls[1].Text != "pdf-processing/scripts/extract.py" {
		t.Errorf("wrapped claim = %+v, want %+v", got, want)
	}
}
