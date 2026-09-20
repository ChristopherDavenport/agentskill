package agentskill_test

import (
	"context"
	"embed"
	"io/fs"
	"strings"
	"testing"

	"github.com/ChristopherDavenport/agentskill"
	"github.com/ChristopherDavenport/agenttool"
	"github.com/ChristopherDavenport/agentturn"
	"github.com/ChristopherDavenport/openresponses"
	"github.com/ChristopherDavenport/openresponses/echo"
)

//go:embed testdata/skills/pdf-processing testdata/skills/minimal
var bundle embed.FS

// TestRunUnderAgentturn wires the prompt and the tool into the loop and
// drives it with the echo adapter, which calls the first tool with the
// user's text as every required argument. The source is an embed.FS,
// so the run touches no disk: the model reaches the skill through the
// tool alone.
func TestRunUnderAgentturn(t *testing.T) {
	fsys, err := fs.Sub(bundle, "testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	c, err := agentskill.Discover(agentskill.Source{FS: fsys, Location: "embed://skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Problems) != 0 {
		t.Fatalf("Problems = %v", c.Problems)
	}
	cfg := agentturn.Config{
		Model:        &echo.Adapter{},
		Instructions: c.Prompt() + "\n\n" + c.Usage(),
		Tools:        []agenttool.Tool{c.Tool()},
	}
	var toolEnds []*agentturn.ToolEnd
	var end *agentturn.RunEnd
	var instructions string
	for ev := range agentturn.Run(context.Background(), nil, openresponses.Items{openresponses.UserText("pdf-processing")}, cfg) {
		switch e := ev.(type) {
		case *agentturn.TurnStart:
			if instructions == "" {
				instructions = e.Request.Instructions
			}
		case *agentturn.ToolEnd:
			toolEnds = append(toolEnds, e)
		case *agentturn.RunEnd:
			end = e
		}
	}
	if end == nil || end.Err != nil {
		t.Fatalf("RunEnd = %+v", end)
	}
	if !strings.Contains(instructions, "<location>\nembed://skills/pdf-processing/SKILL.md\n</location>") {
		t.Errorf("instructions do not list the skill:\n%s", instructions)
	}
	if len(toolEnds) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolEnds))
	}
	te := toolEnds[0]
	if te.Name != agentskill.ToolName || te.Err != nil {
		t.Fatalf("ToolEnd = %+v", te)
	}
	got := te.Result.Output.String()
	if !strings.Contains(got, "# PDF processing") || !strings.Contains(got, "- reference.md (38 bytes)") {
		t.Errorf("tool output = %q", got)
	}
	// The transcript records the call and the output as ordinary items.
	var sawCall, sawOutput bool
	for _, item := range end.Items {
		switch v := item.(type) {
		case *openresponses.FunctionCall:
			sawCall = v.Name == agentskill.ToolName
		case *openresponses.FunctionCallOutput:
			sawOutput = strings.Contains(v.Output.String(), "# PDF processing")
		}
	}
	if !sawCall || !sawOutput {
		t.Errorf("transcript missing the call (%v) or its output (%v)", sawCall, sawOutput)
	}
}
