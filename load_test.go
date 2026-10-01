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

// TestSkillFileLayouts pins which file is the skill file, for Load and
// Discover alike, over an exact fstest.MapFS and over the same map
// behind caseInsensitiveFS. Both must give one answer, the exact one,
// or a tree holds a skill on macOS that does not exist on Linux (#11).
func TestSkillFileLayouts(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		// want is the skill's Location, or "" for no skill.
		want string
		// files is what Files reports for a skill that loads.
		wantFiles []string
		// problem is the key of the one problem Discover reports.
		problem string
	}{
		{name: "uppercase", files: []string{"SKILL.md"}, want: "/s/x/SKILL.md"},
		{name: "lowercase", files: []string{"skill.md"}, want: "/s/x/skill.md"},
		{name: "both", files: []string{"SKILL.md", "skill.md"}, want: "/s/x/SKILL.md"},
		{name: "mixed case", files: []string{"Skill.md", "notes.md"}, problem: "/s/x/Skill.md"},
		{name: "upper extension", files: []string{"SKILL.MD"}, problem: "/s/x/SKILL.MD"},
		{name: "mixed case beside uppercase", files: []string{"SKILL.md", "Skill.md"}, want: "/s/x/SKILL.md", wantFiles: []string{"Skill.md"}},
		{name: "neither", files: []string{"README.md"}},
	}
	for _, tt := range tests {
		m := fstest.MapFS{}
		for _, f := range tt.files {
			m["x/"+f] = &fstest.MapFile{Data: []byte("---\nname: x\ndescription: d\n---\nBody.\n")}
		}
		sub := fstest.MapFS{}
		for p, f := range m {
			sub[strings.TrimPrefix(p, "x/")] = f
		}
		hosts := []struct {
			name      string
			root, dir fs.FS
		}{
			{"exact", m, sub},
			{"case-insensitive", caseInsensitiveFS{m}, caseInsensitiveFS{sub}},
		}
		for _, h := range hosts {
			t.Run(tt.name+"/"+h.name, func(t *testing.T) {
				s, err := Load(h.dir, "/s/x")
				switch {
				case tt.want == "" && err != ErrNoSkillFile:
					t.Errorf("Load err = %v, want %v", err, ErrNoSkillFile)
				case tt.want != "" && err != nil:
					t.Errorf("Load err = %v", err)
				case tt.want != "":
					if s.Location != tt.want {
						t.Errorf("Load Location = %q, want %q", s.Location, tt.want)
					}
					files, err := s.Files()
					if err != nil {
						t.Fatal(err)
					}
					if strings.Join(files, ",") != strings.Join(tt.wantFiles, ",") {
						t.Errorf("Files = %v, want %v", files, tt.wantFiles)
					}
				}

				c, err := Discover(Source{FS: h.root, Location: "/s"})
				if err != nil {
					t.Fatal(err)
				}
				var got string
				if len(c.Skills) == 1 {
					got = c.Skills[0].Location
				} else if len(c.Skills) > 1 {
					t.Fatalf("Skills = %d, want at most one", len(c.Skills))
				}
				if got != tt.want {
					t.Errorf("Discover Location = %q, want %q", got, tt.want)
				}
				var keys []string
				for k := range c.Problems {
					keys = append(keys, k)
				}
				if tt.problem == "" && len(keys) != 0 || tt.problem != "" && (len(keys) != 1 || keys[0] != tt.problem) {
					t.Errorf("Problems = %v, want only %q", c.Problems, tt.problem)
				}
				if p := c.Problems[tt.problem]; tt.problem != "" && (len(p) != 1 || p[0].Severity != Warning) {
					t.Errorf("Problems[%s] = %v, want one warning", tt.problem, p)
				}
			})
		}
	}
}
