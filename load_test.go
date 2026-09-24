package agentskill

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// caseInsensitiveFS is an fs.FS whose lookups ignore case, the way APFS
// and HFS+ on macOS and NTFS on Windows do, while its directory listing
// reports names as they are really spelled.
//
// CI runs on Linux, where the file system is case-sensitive and a probe
// for the wrong spelling simply fails. Without this, the behaviour below
// is only ever exercised on a maintainer's laptop — which is where it
// was found.
type caseInsensitiveFS struct{ m fstest.MapFS }

// resolve maps a requested name onto the one that is really there,
// preferring an exact match, as a case-insensitive file system does.
func (c caseInsensitiveFS) resolve(name string) string {
	if _, ok := c.m[name]; ok {
		return name
	}
	for have := range c.m {
		if strings.EqualFold(have, name) {
			return have
		}
	}
	return name
}

func (c caseInsensitiveFS) Open(name string) (fs.File, error) {
	return c.m.Open(c.resolve(name))
}

// ReadDir is exact: a case-insensitive file system stores the name it
// was given and lists it back unchanged.
func (c caseInsensitiveFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return c.m.ReadDir(name)
}

const lowercaseSkill = "---\nname: lowercase-skill-md\ndescription: Uses the lowercase file name.\n---\nBody.\n"

// TestLoadReportsTheSpellingOnDisk covers a skill whose file is named
// skill.md read through a case-insensitive file system. Reading
// "SKILL.md" succeeds there, so a loader that reports the name it probed
// with puts a spelling in Location that does not exist — and a consumer
// that carries that path to a case-sensitive host cannot open it.
func TestLoadReportsTheSpellingOnDisk(t *testing.T) {
	fsys := caseInsensitiveFS{fstest.MapFS{
		"skill.md": &fstest.MapFile{Data: []byte(lowercaseSkill)},
	}}

	s, err := Load(fsys, "skills/lowercase-skill-md")
	if err != nil {
		t.Fatal(err)
	}
	if want := "skills/lowercase-skill-md/skill.md"; s.Location != want {
		t.Errorf("Location = %q, want %q", s.Location, want)
	}
}

// TestLoadPrefersTheUppercaseSpelling holds the order in skillFiles: with
// both spellings present, SKILL.md is the one read.
func TestLoadPrefersTheUppercaseSpelling(t *testing.T) {
	fsys := caseInsensitiveFS{fstest.MapFS{
		"SKILL.md": &fstest.MapFile{Data: []byte(lowercaseSkill)},
		"skill.md": &fstest.MapFile{Data: []byte(lowercaseSkill)},
	}}

	s, err := Load(fsys, "skills/lowercase-skill-md")
	if err != nil {
		t.Fatal(err)
	}
	if want := "skills/lowercase-skill-md/SKILL.md"; s.Location != want {
		t.Errorf("Location = %q, want %q", s.Location, want)
	}
}

// unlistableFS serves files but does not implement ReadDir, which an
// adapter over a remote source need not. readSkillFile falls back to
// probing there, and the skill still loads.
type unlistableFS struct{ m fstest.MapFS }

func (u unlistableFS) Open(name string) (fs.File, error) { return u.m.Open(name) }

func TestLoadFallsBackWhenTheSourceCannotBeListed(t *testing.T) {
	fsys := unlistableFS{fstest.MapFS{
		"skill.md": &fstest.MapFile{Data: []byte(lowercaseSkill)},
	}}

	s, err := Load(fsys, "skills/lowercase-skill-md")
	if err != nil {
		t.Fatal(err)
	}
	if want := "skills/lowercase-skill-md/skill.md"; s.Location != want {
		t.Errorf("Location = %q, want %q", s.Location, want)
	}
}

// TestLoadWithoutASkillFile is the empty case: a directory that holds no
// skill file at either spelling is not a skill.
func TestLoadWithoutASkillFile(t *testing.T) {
	fsys := caseInsensitiveFS{fstest.MapFS{
		"README.md": &fstest.MapFile{Data: []byte("not a skill")},
	}}

	if _, err := Load(fsys, "skills/notes"); err != ErrNoSkillFile {
		t.Errorf("err = %v, want %v", err, ErrNoSkillFile)
	}
}
