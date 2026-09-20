package agentskill

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	if got := c.Names(); len(got) != 3 || got[0] != "Wrong" || got[1] != "good" || got[2] != "" {
		t.Errorf("Names() = %q", got)
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
