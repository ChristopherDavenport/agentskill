package agentskill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ChristopherDavenport/agenttool"
)

// The tree under testdata/ref is the round 2 design study's fixture
// set, and testdata/ref/golden holds what the reference CLI printed
// over it: skills-ref 0.1.1, run once with uvx and captured, so these
// tests are offline. The golden files are the reference's bytes and
// not ours; go test -update does not write them, and changing one
// means running the reference again.
//
// <skills> stands for the absolute path of testdata/ref/skills, so the
// captures are the same on every machine.

// refDirs returns the absolute, symlink-resolved fixture paths: the
// base the goldens are written against, and the two sources a product
// would list, most specific first.
func refDirs(t *testing.T) (base, project, user string) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "ref", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	base, err = filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	return base, filepath.Join(base, "project"), filepath.Join(base, "user")
}

// refCatalog discovers the fixture skills the way a product would.
func refCatalog(t *testing.T) (*Catalog, string) {
	t.Helper()
	base, project, user := refDirs(t)
	c, err := DiscoverDirs(project, user)
	if err != nil {
		t.Fatal(err)
	}
	return c, base
}

// refGolden reads one capture with the fixture path substituted in.
func refGolden(t *testing.T, name, base string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ref", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), skillsPlaceholder, base)
}

// TestPromptMatchesReference is the conformance claim: over the skills
// the reference renders, Prompt is byte for byte skills-ref 0.1.1's
// to-prompt, HTML escaping and all. The fixture set holds one skill
// the reference refuses to render, project/nameless, which is why the
// catalogue has one more skill than the block has entries.
func TestPromptMatchesReference(t *testing.T) {
	c, base := refCatalog(t)
	if len(c.Skills) != 8 {
		t.Fatalf("Skills = %d, want 8", len(c.Skills))
	}
	if len(c.Listed()) != 7 {
		t.Fatalf("Listed() = %d, want 7", len(c.Listed()))
	}
	got := c.Prompt()
	want := strings.TrimRight(refGolden(t, "ref-to-prompt.txt", base), "\n")
	if got != want {
		t.Errorf("Prompt() differs from skills-ref to-prompt:\n%s", firstDiff(got, want))
	}
}

// TestNamelessSkillIsNotOffered: a skill with no name is loaded and
// reported, and is offered nowhere. The prompt block costs tokens on
// every turn, so an entry the model cannot call is worse than no
// entry, and the tool's own list of available skills used to show the
// empty name between two commas.
func TestNamelessSkillIsNotOffered(t *testing.T) {
	c, base := refCatalog(t)
	nameless := filepath.Join(base, "project", "nameless", "SKILL.md")

	var found *Skill
	for _, s := range c.Skills {
		if s.Location == nameless {
			found = s
		}
	}
	if found == nil {
		t.Fatal("the nameless skill is not in Skills; a broken skill must still be listed")
	}
	if len(c.Problems[nameless]) != 2 {
		t.Errorf("Problems[nameless] = %v, want the empty name and the empty description", c.Problems[nameless])
	}
	for _, s := range c.Listed() {
		if s == found {
			t.Error("Listed() offers the nameless skill")
		}
	}
	for _, n := range c.Names() {
		if n == "" {
			t.Errorf("Names() = %q, which offers a name the model cannot use", c.Names())
		}
	}
	if strings.Contains(c.Prompt(), "<name>\n\n</name>") {
		t.Error("Prompt() renders an empty name")
	}
	for _, name := range []string{"", "nameless"} {
		if s, ok := c.Lookup(name); ok {
			t.Errorf("Lookup(%q) = %s, want not found", name, s.Location)
		}
	}

	// The tool refuses it too, and its list of what the model could
	// have used holds no empty name.
	_, err := c.Tool().Execute(context.Background(), agenttool.Call{ID: "c1", Args: json.RawMessage(`{"name":""}`)})
	if err == nil {
		t.Fatal("the tool served a skill with no name")
	}
	msg := err.Error()
	list, _, _ := strings.Cut(strings.TrimPrefix(msg, `unknown skill ""; available skills: `), "\n")
	for _, n := range strings.Split(list, ", ") {
		if n == "" {
			t.Errorf("the tool lists an empty skill name: %s", msg)
		}
	}
}

// firstDiff reports the first line at which two renderings part.
func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return fmt.Sprintf("line %d:\n  ours: %q\n  ref:  %q", i+1, g[i], w[i])
		}
	}
	if len(g) != len(w) {
		return fmt.Sprintf("ours has %d lines, the reference %d", len(g), len(w))
	}
	return "no line differs"
}

// refVerdicts parses a captured validate run into a map from fixture
// directory to the reference's error messages, empty when it called
// the skill valid.
func refVerdicts(t *testing.T, name, base string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	var cur string
	for _, line := range strings.Split(refGolden(t, name, base), "\n") {
		switch {
		case strings.HasPrefix(line, "### "):
			cur = strings.Fields(strings.TrimPrefix(line, "### "))[0]
			out[cur] = nil
		case strings.HasPrefix(line, "  - "):
			out[cur] = append(out[cur], strings.TrimPrefix(line, "  - "))
		}
	}
	return out
}

// TestDuplicateFrontmatterKeyRefused: a key written twice used to load
// silently with the last value winning, where the reference's
// strictyaml refuses the file. The two readers of one format then
// disagreed about what a skill says, not merely about whether it is
// well formed, and the shape that matters is a first allowed-tools or
// description line that a reviewer approves and a second that the
// model is given.
func TestDuplicateFrontmatterKeyRefused(t *testing.T) {
	base, _, _ := refDirs(t)
	dir := filepath.Join(base, "yaml", "duplicate-key")

	s, err := LoadDir(dir)
	if err == nil {
		t.Fatalf("loaded a file with two descriptions; the skill carries %q", s.Description)
	}
	// The line is counted within the frontmatter, as every other YAML
	// message from this package is.
	const want = `Invalid YAML in frontmatter: line 3: duplicate key "description"`
	if err.Error() != want {
		t.Errorf("LoadDir() error = %q, want %q", err, want)
	}
	if ref := refVerdicts(t, "ref-validate-yaml.txt", base)["yaml/duplicate-key"]; len(ref) == 0 {
		t.Error("the reference accepts the file; the capture under testdata/ref/golden says it refuses it")
	}

	// Discovery reports it under its location and offers nothing.
	c, err := DiscoverDirs(filepath.Join(base, "yaml"))
	if err != nil {
		t.Fatal(err)
	}
	loc := filepath.Join(dir, "SKILL.md")
	if p := c.Problems[loc]; len(p) != 1 || p[0].Message != want {
		t.Errorf("Problems[%s] = %v, want the duplicate key", loc, p)
	}
	for _, skill := range c.Skills {
		if skill.Location == loc {
			t.Error("a file with a duplicate key is in Skills")
		}
	}
}

// ourVerdict is this module's error-severity messages for one fixture
// directory, in the shape the reference prints: a load failure is its
// one message, and a skill with nothing wrong is "valid".
func ourVerdict(t *testing.T, dir string) []string {
	t.Helper()
	s, err := LoadDir(dir)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, p := range s.Validate() {
		if p.Severity == Error {
			out = append(out, p.Message)
		}
	}
	if len(out) == 0 {
		return []string{"valid"}
	}
	return out
}

// TestValidateVerdictsMatchReference pins the README's claim that
// Validate reports the reference validator's verdicts and wording:
// over the rules of the specification, every fixture's verdict is
// message for message what skills-ref 0.1.1 printed, except the one
// divergence the README names. A new divergence fails here.
//
// The YAML dialect is a separate capture, compared on the verdict
// alone, because there the message text is the parser's own; see
// TestYAMLDialectBounds.
func TestValidateVerdictsMatchReference(t *testing.T) {
	base, _, _ := refDirs(t)
	// The reason the listed fixture is allowed to differ; every other
	// fixture must agree message for message.
	diverges := map[string]string{
		"project/wide-open": "an empty allowed-tools specifier is refused here and accepted there, deliberately",
	}
	ref := refVerdicts(t, "ref-validate.txt", base)
	if len(ref) != 12 {
		t.Fatalf("the capture holds %d fixtures, want 12", len(ref))
	}
	for dir, want := range ref {
		t.Run(dir, func(t *testing.T) {
			if len(want) == 0 {
				want = []string{"valid"}
			}
			got := ourVerdict(t, filepath.Join(base, filepath.FromSlash(dir)))
			agree := sameVerdicts(got, want)
			why, listed := diverges[dir]
			switch {
			case agree && listed:
				t.Errorf("%s now agrees with the reference; drop it from the divergence list", dir)
			case agree:
			case listed:
				t.Logf("diverges by design (%s)\n  ours: %s\n  ref:  %s", why, strings.Join(got, " | "), strings.Join(want, " | "))
			default:
				t.Errorf("verdicts differ and the divergence is not one the README names\n  ours: %s\n  ref:  %s",
					strings.Join(got, " | "), strings.Join(want, " | "))
			}
		})
	}
}

// sameVerdicts compares two sets of messages, order aside.
func sameVerdicts(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	return slices.Equal(x, y)
}

// TestYAMLDialectBounds records where the two readers part over the
// YAML they accept, which is the class of input the README's
// conformance claim excludes: the reference's strictyaml forbids flow
// style, anchors and aliases where go.yaml.in/yaml/v3 allows them, and
// the reference normalises to NFKC where this module compares as
// written. Only the verdict is compared, because the message text on
// either side is its own parser's. The duplicate key used to be in
// this class and is not any more; anchored agrees by accident, the
// reference refusing the anchor and this module the shape of the value
// the alias resolved to.
func TestYAMLDialectBounds(t *testing.T) {
	base, _, _ := refDirs(t)
	ref := refVerdicts(t, "ref-validate-yaml.txt", base)
	tests := []struct {
		dir      string
		ours     bool // does this module accept the file?
		refTakes bool // does the reference?
	}{
		{dir: "yaml/file-tools", ours: false, refTakes: true},
		{dir: "yaml/flow-metadata", ours: true, refTakes: false},
		{dir: "yaml/anchored", ours: false, refTakes: false},
		{dir: "yaml/duplicate-key", ours: false, refTakes: false},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			got := sameVerdicts(ourVerdict(t, filepath.Join(base, filepath.FromSlash(tt.dir))), []string{"valid"})
			if got != tt.ours {
				t.Errorf("this module accepts %s = %v, want %v", tt.dir, got, tt.ours)
			}
			if refOK := len(ref[tt.dir]) == 0; refOK != tt.refTakes {
				t.Errorf("skills-ref accepts %s = %v, want %v", tt.dir, refOK, tt.refTakes)
			}
		})
	}
}
