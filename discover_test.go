package agentskill

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ChristopherDavenport/agenttool"
)

func skillFS(name, description string, extra map[string]string) fstest.MapFS {
	m := fstest.MapFS{
		name + "/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: " + name + "\ndescription: " + description + "\n---\nBody of " + name + "\n")},
	}
	for p, content := range extra {
		m[name+"/"+p] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

func merge(fss ...fstest.MapFS) fstest.MapFS {
	out := fstest.MapFS{}
	for _, f := range fss {
		for k, v := range f {
			out[k] = v
		}
	}
	return out
}

func TestDiscoverPrecedence(t *testing.T) {
	first := Source{FS: merge(skillFS("shared", "from first", nil), skillFS("only-first", "one", nil)), Location: "mcp://first"}
	second := Source{FS: merge(skillFS("shared", "from second", nil), skillFS("only-second", "two", nil),
		fstest.MapFS{"not-a-skill/README.md": &fstest.MapFile{Data: []byte("x")}, "loose-file.md": &fstest.MapFile{Data: []byte("x")}}), Location: "/second"}
	c, err := Discover(first, second)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"only-first", "shared", "only-second"}
	if got := c.Names(); len(got) != len(wantNames) {
		t.Fatalf("Names() = %v, want %v", got, wantNames)
	} else {
		for i := range got {
			if got[i] != wantNames[i] {
				t.Fatalf("Names() = %v, want %v", got, wantNames)
			}
		}
	}
	s, ok := c.Lookup("shared")
	if !ok || s.Description != "from first" {
		t.Errorf("Lookup(shared) = %+v, %v; want the first source's", s, ok)
	}
	if s.Location != "mcp://first/shared/SKILL.md" {
		t.Errorf("Location = %q", s.Location)
	}
	if len(c.Shadowed) != 1 || c.Shadowed[0].Location != "/second/shared/SKILL.md" {
		t.Errorf("Shadowed = %v", c.Shadowed)
	}
	if len(c.Problems) != 0 {
		t.Errorf("Problems = %v, want none", c.Problems)
	}
	want := "<available_skills>\n<skill>\n<name>\nonly-first\n</name>\n<description>\none\n</description>\n<location>\nmcp://first/only-first/SKILL.md\n</location>\n</skill>\n" +
		"<skill>\n<name>\nshared\n</name>\n<description>\nfrom first\n</description>\n<location>\nmcp://first/shared/SKILL.md\n</location>\n</skill>\n" +
		"<skill>\n<name>\nonly-second\n</name>\n<description>\ntwo\n</description>\n<location>\n/second/only-second/SKILL.md\n</location>\n</skill>\n</available_skills>"
	if got := c.Prompt(); got != want {
		t.Errorf("Prompt() =\n%s\nwant\n%s", got, want)
	}
}

func TestDiscoverBrokenSkillIsListed(t *testing.T) {
	fsys := merge(
		fstest.MapFS{"bad/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: Wrong\ndescription: d\n---\n")}},
		fstest.MapFS{"broken/SKILL.md": &fstest.MapFile{Data: []byte("no frontmatter")}},
		fstest.MapFS{"nameless/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: d\n---\n")}},
		skillFS("good", "d", nil),
	)
	c, err := Discover(Source{FS: fsys, Location: "/src"})
	if err != nil {
		t.Fatal(err)
	}
	// A skill with a bad name is still offered, as the reference
	// renders it; one with no name at all is not, because there is
	// nothing the model could call it.
	if got := c.Names(); len(got) != 2 || got[0] != "Wrong" || got[1] != "good" {
		t.Errorf("Names() = %q", got)
	}
	if len(c.Skills) != 3 {
		t.Errorf("Skills = %d, want the nameless skill kept", len(c.Skills))
	}
	if p := c.Problems["/src/nameless/SKILL.md"]; len(p) != 1 {
		t.Errorf("nameless problems = %v, want the missing name reported", p)
	}
	if p := c.Problems["/src/broken/SKILL.md"]; len(p) != 1 || p[0].Message != ErrNoFrontmatter.Error() {
		t.Errorf("broken problems = %v", p)
	}
	if p := c.Problems["/src/bad/SKILL.md"]; len(p) != 2 {
		t.Errorf("bad problems = %v, want lowercase and directory mismatch", p)
	}
	if _, ok := c.Lookup("nameless"); ok {
		t.Error("a nameless skill must not be found by name")
	}
}

func TestDiscoverSourceErrors(t *testing.T) {
	if _, err := Discover(Source{Location: "x"}); err == nil {
		t.Error("nil FS: want error")
	}
	if _, err := Discover(Source{FS: fstest.MapFS{}, Location: "empty"}); err != nil {
		t.Errorf("empty source: %v", err)
	}
	if _, err := DiscoverDirs(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing dir: want error")
	}
	if _, err := Dir("load.go"); err == nil {
		t.Error("file as dir: want error")
	}
}

func TestPromptEscapes(t *testing.T) {
	c := &Catalog{Skills: []*Skill{{Name: "a&b", Description: `<x> "y" 'z'`, Location: "/l/<not escaped>"}}}
	want := "<available_skills>\n<skill>\n<name>\na&amp;b\n</name>\n<description>\n&lt;x&gt; &quot;y&quot; &#x27;z&#x27;\n</description>\n<location>\n/l/<not escaped>\n</location>\n</skill>\n</available_skills>"
	if got := c.Prompt(); got != want {
		t.Errorf("Prompt() =\n%s\nwant\n%s", got, want)
	}
	if got := (&Catalog{}).Prompt(); got != "<available_skills>\n</available_skills>" {
		t.Errorf("empty Prompt() = %q", got)
	}
	if (&Catalog{}).Usage() == "" {
		t.Error("Usage() is empty")
	}
}

// TestDirSymlinkGuard: a link inside the directory is followed, one
// leading outside is refused, through Open, ReadFile, Stat, ReadDir
// and a fs.Sub over the source.
func TestDirSymlinkGuard(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(root, "linked")
	if err := os.MkdirAll(filepath.Join(skill, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: linked\ndescription: d\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "sub", "inside.md"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(skill, "sub", "inside.md"), filepath.Join(skill, "ok.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(skill, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(skill, "escapedir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skill, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}

	c, err := DiscoverDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	// "alias" links inside the source, so it is a second directory for
	// the same skill. Directory order decides: alias sorts first and
	// wins, with a directory-mismatch problem; linked is shadowed.
	if got := c.Names(); len(got) != 1 || got[0] != "linked" {
		t.Fatalf("Names() = %v", got)
	}
	if len(c.Skills) != 1 || c.Skills[0].DirName != "alias" {
		t.Fatalf("Skills = %v, want the alias directory", c.Skills)
	}
	if len(c.Shadowed) != 1 || c.Shadowed[0].DirName != "linked" {
		t.Fatalf("Shadowed = %v, want the linked directory", c.Shadowed)
	}
	if p := c.Problems[c.Skills[0].Location]; len(p) != 1 || !strings.Contains(p[0].Message, "Directory name 'alias'") {
		t.Errorf("alias problems = %v", p)
	}
	s := c.Shadowed[0]

	data, err := fs.ReadFile(s.FS, "ok.md")
	if err != nil || string(data) != "inside" {
		t.Errorf("ReadFile(ok.md) = %q, %v", data, err)
	}
	for _, name := range []string{"escape.md", "escapedir/secret.md"} {
		if _, err := s.Open(name); !errors.Is(err, ErrOutside) {
			t.Errorf("Open(%s) error = %v, want ErrOutside", name, err)
		}
		if _, err := fs.ReadFile(s.FS, name); !errors.Is(err, ErrOutside) {
			t.Errorf("ReadFile(%s) error = %v, want ErrOutside", name, err)
		}
		if _, err := fs.Stat(s.FS, name); !errors.Is(err, ErrOutside) {
			t.Errorf("Stat(%s) error = %v, want ErrOutside", name, err)
		}
	}
	if _, err := fs.ReadDir(s.FS, "escapedir"); !errors.Is(err, ErrOutside) {
		t.Errorf("ReadDir(escapedir) error = %v, want ErrOutside", err)
	}
	if _, err := s.Open("../other"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("Open(../other) error = %v, want ErrInvalid", err)
	}
	if _, err := s.Open("missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(missing.md) error = %v, want ErrNotExist", err)
	}
	files, err := s.Files()
	if err != nil {
		t.Fatal(err)
	}
	// Files lists what is there, links included; opening decides.
	want := []string{"escape.md", "escapedir", "ok.md", "sub/inside.md"}
	if len(files) != len(want) {
		t.Fatalf("Files() = %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("Files() = %v, want %v", files, want)
		}
	}
	if _, err := (&Skill{}).Open("x"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open on a skill without FS: %v", err)
	}
	if files, err := (&Skill{}).Files(); err != nil || files != nil {
		t.Errorf("Files on a skill without FS: %v, %v", files, err)
	}
}

// An unlisted skill keeps its place in Skills but claims no name, so a
// later source's skill of its directory name is listed, not shadowed
// (#19).
func TestDiscoverUnlistedClaimsNoName(t *testing.T) {
	tests := []struct {
		name string
		md   string
	}{
		{"no name", "---\ndescription: d\n---\n"},
		{"empty name and description", "---\nname: \"\"\ndescription: \"\"\n---\n"},
		{"no description", "---\nname: deploy\n---\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := Source{FS: fstest.MapFS{"deploy/SKILL.md": &fstest.MapFile{Data: []byte(tt.md)}}, Location: "/repo"}
			user := Source{FS: skillFS("deploy", "Deploy, my way.", nil), Location: "/home"}
			c, err := Discover(project, user)
			if err != nil {
				t.Fatal(err)
			}
			s, ok := c.Lookup("deploy")
			if !ok || s.Location != "/home/deploy/SKILL.md" {
				t.Errorf("Lookup(deploy) = %v, %v; want the user's skill", s, ok)
			}
			if len(c.Skills) != 2 || len(c.Shadowed) != 0 {
				t.Errorf("Skills = %d, Shadowed = %d; want both listed in Skills and none shadowed", len(c.Skills), len(c.Shadowed))
			}
		})
	}
}

// Two sources with one Location are refused, since their skills of one
// directory name would share a location and a key in Problems (#20).
func TestDiscoverSharedLocation(t *testing.T) {
	a := Source{FS: skillFS("foo", "first", nil), Location: "builtin"}
	b := Source{FS: fstest.MapFS{"foo/SKILL.md": &fstest.MapFile{Data: []byte("not a skill")}}, Location: "builtin"}
	other := Source{FS: fstest.MapFS{}, Location: "other"}
	_, err := Discover(a, other, b)
	if err == nil || !strings.Contains(err.Error(), "sources 0 and 2 share the location \"builtin\"") {
		t.Errorf("err = %v, want the two indices and the location", err)
	}
	dir := t.TempDir()
	if _, err := DiscoverDirs(dir, dir); err == nil {
		t.Error("one directory twice: want error")
	}
}

// A source with a Qualifier lists a skill of a taken name under the
// qualified name rather than shadowing it (#18).
func TestDiscoverQualifier(t *testing.T) {
	root := Source{FS: merge(skillFS("deploy", "Deploy the monorepo.", nil), skillFS("lint", "Lint.", nil)), Location: "/repo/.claude/skills"}
	tests := []struct {
		name         string
		sources      []Source
		wantNames    []string
		wantShadowed []string
	}{
		{
			name: "no qualifier shadows",
			sources: []Source{root,
				{FS: skillFS("deploy", "web", nil), Location: "/repo/apps/web/.claude/skills"}},
			wantNames:    []string{"deploy", "lint"},
			wantShadowed: []string{"/repo/apps/web/.claude/skills/deploy/SKILL.md"},
		},
		{
			name: "a taken name is qualified and a free one is not",
			sources: []Source{root,
				{FS: merge(skillFS("deploy", "web", nil), skillFS("serve", "Serve.", nil)), Location: "/repo/apps/web/.claude/skills", Qualifier: "apps/web"}},
			wantNames: []string{"deploy", "lint", "apps/web:deploy", "serve"},
		},
		{
			name: "a taken qualified name is shadowed",
			sources: []Source{root,
				{FS: skillFS("deploy", "web", nil), Location: "/a", Qualifier: "plugin"},
				{FS: skillFS("deploy", "web again", nil), Location: "/b", Qualifier: "plugin"}},
			wantNames:    []string{"deploy", "lint", "plugin:deploy"},
			wantShadowed: []string{"/b/deploy/SKILL.md"},
		},
		{
			name: "the first of a name is never qualified",
			sources: []Source{
				{FS: skillFS("deploy", "plugin", nil), Location: "/p", Qualifier: "plugin"},
				root},
			wantNames:    []string{"deploy", "lint"},
			wantShadowed: []string{"/repo/.claude/skills/deploy/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Discover(tt.sources...)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.Names(); strings.Join(got, ",") != strings.Join(tt.wantNames, ",") {
				t.Errorf("Names() = %v, want %v", got, tt.wantNames)
			}
			var shadowed []string
			for _, s := range c.Shadowed {
				if s.Qualifier != "" {
					t.Errorf("shadowed %s has Qualifier %q, want none", s.Location, s.Qualifier)
				}
				shadowed = append(shadowed, s.Location)
			}
			if strings.Join(shadowed, ",") != strings.Join(tt.wantShadowed, ",") {
				t.Errorf("Shadowed = %v, want %v", shadowed, tt.wantShadowed)
			}
			for _, n := range tt.wantNames {
				s, ok := c.Lookup(n)
				if !ok || s.ListedName() != n {
					t.Errorf("Lookup(%q) = %v, %v", n, s, ok)
				}
			}
		})
	}
}

// A qualified skill is rendered, looked up and recorded under its
// qualified name, and validated under its own.
func TestQualifiedSkillThroughPromptAndTool(t *testing.T) {
	root := Source{FS: skillFS("deploy", "Deploy the monorepo.", nil), Location: "/repo"}
	nested := Source{FS: skillFS("deploy", "Deploy the web app.", nil), Location: "/repo/apps/web", Qualifier: "apps/web"}
	c, err := Discover(root, nested)
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Prompt(); !strings.Contains(p, "<name>\napps/web:deploy\n</name>\n<description>\nDeploy the web app.\n</description>\n<location>\n/repo/apps/web/deploy/SKILL.md\n</location>") {
		t.Errorf("Prompt() =\n%s\nwant the nested skill under its qualified name", p)
	}
	if len(c.Problems) != 0 {
		t.Errorf("Problems = %v, want a qualified skill valid under its own name", c.Problems)
	}
	res, err := c.Tool().Execute(context.Background(), agenttool.Call{ID: "c1", Args: json.RawMessage(`{"name":"apps/web:deploy"}`)})
	if err != nil {
		t.Fatal(err)
	}
	read, ok := res.Details.(Read)
	if !ok || read.Name != "apps/web:deploy" || read.Location != "/repo/apps/web/deploy/SKILL.md" {
		t.Errorf("Details = %#v, want the nested skill under its qualified name", res.Details)
	}
	if _, err := c.Tool().Execute(context.Background(), agenttool.Call{ID: "c2", Args: json.RawMessage(`{"name":"apps/web:deploy","path":"missing.md"}`)}); err == nil || !strings.Contains(err.Error(), `skill "apps/web:deploy" has no file`) {
		t.Errorf("missing file err = %v", err)
	}
}
