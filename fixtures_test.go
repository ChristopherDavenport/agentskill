package agentskill

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

//go:embed testdata/skills
var embedded embed.FS

// skillsPlaceholder stands in for the absolute fixture directory in
// golden files, so they are the same on every machine.
const skillsPlaceholder = "<skills>"

func skillsDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// fixtureNames lists the fixture directories.
func fixtureNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// mapFS copies a tree into a fstest.MapFS.
func mapFS(t *testing.T, fsys fs.FS) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." {
				m[p] = &fstest.MapFile{Mode: fs.ModeDir}
			}
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		m[p] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// loaders are the three ways a fixture is read; every one must give
// the same skill.
type loader struct {
	name string
	load func(t *testing.T, fixture string) (*Skill, error)
}

func loaders(t *testing.T) []loader {
	t.Helper()
	dir := skillsDir(t)
	sub, err := fs.Sub(embedded, "testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	mapped := mapFS(t, sub)
	return []loader{
		{"dir", func(t *testing.T, fixture string) (*Skill, error) {
			return LoadDir(filepath.Join(dir, fixture))
		}},
		{"embed", func(t *testing.T, fixture string) (*Skill, error) {
			fsys, err := fs.Sub(sub, fixture)
			if err != nil {
				t.Fatal(err)
			}
			return Load(fsys, dir+"/"+fixture)
		}},
		{"mapfs", func(t *testing.T, fixture string) (*Skill, error) {
			fsys, err := fs.Sub(mapped, fixture)
			if err != nil {
				t.Fatal(err)
			}
			return Load(fsys, dir+"/"+fixture)
		}},
	}
}

// report renders what a fixture loads to, for the golden file.
func report(s *Skill, err error) string {
	var b strings.Builder
	if err != nil {
		fmt.Fprintf(&b, "load error: %s\n", err)
		return b.String()
	}
	problems := s.Validate()
	if len(problems) == 0 {
		b.WriteString("valid\n")
	}
	for _, p := range problems {
		fmt.Fprintf(&b, "%s\n", p)
	}
	files, err := s.Files()
	if err != nil {
		fmt.Fprintf(&b, "files error: %s\n", err)
	}
	for _, f := range files {
		fmt.Fprintf(&b, "file: %s\n", f)
	}
	return b.String()
}

func TestFixtures(t *testing.T) {
	dir := skillsDir(t)
	for _, fixture := range fixtureNames(t) {
		t.Run(fixture, func(t *testing.T) {
			var reports []string
			var skills []*Skill
			for _, l := range loaders(t) {
				s, err := l.load(t, fixture)
				reports = append(reports, report(s, err))
				skills = append(skills, s)
			}
			for i := 1; i < len(reports); i++ {
				if reports[i] != reports[0] {
					t.Errorf("loader %d reports\n%s\nloader 0 reports\n%s", i, reports[i], reports[0])
				}
			}
			for i := 1; i < len(skills); i++ {
				if !sameSkill(skills[0], skills[i]) {
					t.Errorf("loader %d loaded\n%#v\nloader 0 loaded\n%#v", i, skills[i], skills[0])
				}
			}
			checkGolden(t, filepath.Join("testdata", "golden", fixture, "report.txt"), reports[0])
			if skills[0] != nil {
				encoded, err := Encode(skills[0])
				if err != nil {
					t.Fatal(err)
				}
				checkGolden(t, filepath.Join("testdata", "golden", fixture, "encode.md"), string(encoded))
				if want := dir + "/" + fixture + "/"; !strings.HasPrefix(skills[0].Location, want) {
					t.Errorf("Location = %q, want prefix %q", skills[0].Location, want)
				}
				if skills[0].DirName != fixture {
					t.Errorf("DirName = %q, want %q", skills[0].DirName, fixture)
				}
			}
		})
	}
}

// sameSkill compares two skills field by field, FS aside.
func sameSkill(a, b *Skill) bool {
	if a == nil || b == nil {
		return a == b
	}
	ac, bc := *a, *b
	ac.FS, bc.FS = nil, nil
	return reflect.DeepEqual(ac, bc)
}

func TestFixtureCatalog(t *testing.T) {
	dir := skillsDir(t)
	c, err := DiscoverDirs("testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(c.Prompt())
	b.WriteString("\n\nproblems:\n")
	locations := make([]string, 0, len(c.Problems))
	for loc := range c.Problems {
		locations = append(locations, loc)
	}
	sort.Strings(locations)
	for _, loc := range locations {
		for _, p := range c.Problems[loc] {
			fmt.Fprintf(&b, "%s: %s\n", strings.TrimPrefix(loc, dir+"/"), p)
		}
	}
	if len(c.Shadowed) != 0 {
		t.Errorf("Shadowed = %d skills, want none", len(c.Shadowed))
	}
	got := strings.ReplaceAll(b.String(), dir, skillsPlaceholder)
	checkGolden(t, filepath.Join("testdata", "golden", "catalog.txt"), got)
}

// checkGolden compares got with the golden file, or rewrites it under
// -update. The absolute fixture directory is replaced by a placeholder
// in both directions.
func checkGolden(t *testing.T, path, got string) {
	t.Helper()
	dir := skillsDir(t)
	got = strings.ReplaceAll(got, dir, skillsPlaceholder)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test . -update)", err)
	}
	if !bytes.Equal(want, []byte(got)) {
		t.Errorf("%s differs from golden:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
