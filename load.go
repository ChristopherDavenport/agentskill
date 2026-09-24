package agentskill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// skillFiles are the names of the skill file, in order of preference.
// The reference reader accepts the lowercase spelling too.
var skillFiles = []string{"SKILL.md", "skill.md"}

// ErrNoSkillFile is returned by [Load] when the root of the file system
// holds no SKILL.md.
//
//lint:ignore ST1005 mirrors the reference validator's wording
var ErrNoSkillFile = errors.New("Missing required file: SKILL.md")

// Load reads the skill at the root of fsys: its SKILL.md, parsed with
// [Parse], attached to fsys so [Skill.Files] and [Skill.Open] reach the
// resource files. location names the skill directory for the prompt;
// the skill's Location is it joined with the name of the skill file,
// and its DirName is its last element, so a name that does not match
// the directory is reported. An empty location leaves DirName empty
// and the match unchecked.
//
// A malformed SKILL.md fails to load. A well-formed one with a bad name
// loads, and [Skill.Validate] says what is wrong, so a product can list
// a broken skill and say why rather than make it vanish.
func Load(fsys fs.FS, location string) (*Skill, error) {
	file, src, err := readSkillFile(fsys)
	if err != nil {
		return nil, err
	}
	s, err := Parse(src)
	if err != nil {
		return nil, err
	}
	s.FS = fsys
	s.Location = joinLocation(location, file)
	s.DirName = baseName(location)
	return s, nil
}

// LoadDir is [Load] over the directory, with the absolute, symlink
// resolved path as the location and the [Dir] guard on its files.
func LoadDir(dir string) (*Skill, error) {
	src, err := Dir(dir)
	if err != nil {
		return nil, err
	}
	return Load(src.FS, src.Location)
}

// readSkillFile returns the name of the skill file, as it is spelled at
// the root of fsys, and its contents.
//
// The name is taken from the directory listing rather than from the name
// the read succeeded under. On a case-insensitive file system — APFS and
// HFS+ on macOS, NTFS on Windows — reading "SKILL.md" succeeds against a
// file that is really named "skill.md", and reporting the probe back
// would put a spelling in Location that does not exist on a
// case-sensitive host. The listing is exact everywhere.
//
// An fs.FS need not implement ReadDir, so a source that cannot be listed
// falls back to probing; there the probed name is the best available.
func readSkillFile(fsys fs.FS) (string, []byte, error) {
	if entries, err := fs.ReadDir(fsys, "."); err == nil {
		present := make(map[string]bool, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				present[e.Name()] = true
			}
		}
		for _, name := range skillFiles {
			if !present[name] {
				continue
			}
			src, err := fs.ReadFile(fsys, name)
			if err != nil {
				return "", nil, fmt.Errorf("agentskill: read %s: %w", name, err)
			}
			return name, src, nil
		}
		return "", nil, ErrNoSkillFile
	}
	for _, name := range skillFiles {
		src, err := fs.ReadFile(fsys, name)
		if err == nil {
			return name, src, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, fmt.Errorf("agentskill: read %s: %w", name, err)
		}
	}
	return "", nil, ErrNoSkillFile
}

// joinLocation appends elem to a location with a forward slash, which
// serves file paths and URLs alike. An empty location yields elem.
func joinLocation(location, elem string) string {
	if location == "" {
		return elem
	}
	return strings.TrimSuffix(location, "/") + "/" + elem
}

// baseName is the last element of a location, "" for an empty one.
func baseName(location string) string {
	if location == "" {
		return ""
	}
	base := path.Base(strings.TrimSuffix(filepath.ToSlash(location), "/"))
	if base == "." || base == "/" {
		return ""
	}
	return base
}

// Files lists every file in the skill's tree except the skill file
// itself, as fs paths relative to the skill's root, sorted. A skill
// with no FS has no files.
func (s *Skill) Files() ([]string, error) {
	if s.FS == nil {
		return nil, nil
	}
	var files []string
	err := fs.WalkDir(s.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isSkillFile(p) {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("agentskill: list files of %s: %w", s.Location, err)
	}
	sort.Strings(files)
	return files, nil
}

// isSkillFile reports whether p is the skill file at the root.
func isSkillFile(p string) bool {
	for _, name := range skillFiles {
		if p == name {
			return true
		}
	}
	return false
}

// Open opens a file in the skill's tree. name must satisfy
// fs.ValidPath, which refuses "..", absolute paths and empty elements,
// so nothing outside the tree can be named.
func (s *Skill) Open(name string) (fs.File, error) {
	if s.FS == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return s.FS.Open(name)
}

// Dir is the [Source] for a local directory: os.DirFS over the
// directory, with its absolute, symlink resolved path as the location,
// so the prompt shows what skills-ref shows. The directory must exist.
//
// Dir adds the one guard os.DirFS lacks: a path that resolves through a
// symlink to somewhere outside the directory is refused with
// [ErrOutside], because a local skill tree may be user-installed and a
// link to /etc inside it must not be readable through the tool. That
// includes a skill directory that is itself a link elsewhere; add the
// link's target as its own Dir source instead.
func Dir(dir string) (Source, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Source{}, fmt.Errorf("agentskill: %w", err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Source{}, fmt.Errorf("agentskill: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Source{}, fmt.Errorf("agentskill: %w", err)
	}
	if !info.IsDir() {
		return Source{}, fmt.Errorf("agentskill: %s: not a directory", dir)
	}
	return Source{FS: &dirFS{root: root, fs: os.DirFS(root)}, Location: root}, nil
}

// ErrOutside is the error of a [Dir] file whose path resolves, through
// a symlink, to a place outside the directory.
var ErrOutside = errors.New("path resolves outside the directory")

// dirFS is os.DirFS with the symlink guard. Every method that takes a
// path resolves it first.
type dirFS struct {
	root string
	fs   fs.FS
}

func (d *dirFS) Open(name string) (fs.File, error) {
	if err := d.check("open", name); err != nil {
		return nil, err
	}
	return d.fs.Open(name)
}

func (d *dirFS) ReadFile(name string) ([]byte, error) {
	if err := d.check("readfile", name); err != nil {
		return nil, err
	}
	return fs.ReadFile(d.fs, name)
}

func (d *dirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := d.check("readdir", name); err != nil {
		return nil, err
	}
	return fs.ReadDir(d.fs, name)
}

func (d *dirFS) Stat(name string) (fs.FileInfo, error) {
	if err := d.check("stat", name); err != nil {
		return nil, err
	}
	return fs.Stat(d.fs, name)
}

// check refuses an invalid path or one that resolves outside root. A
// path that does not exist passes, so the underlying call returns the
// usual not-exist error.
func (d *dirFS) check(op, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	full := filepath.Join(d.root, filepath.FromSlash(name))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return &fs.PathError{Op: op, Path: name, Err: err}
	}
	if resolved != d.root && !strings.HasPrefix(resolved, d.root+string(filepath.Separator)) {
		return &fs.PathError{Op: op, Path: name, Err: ErrOutside}
	}
	return nil
}

var (
	_ fs.FS         = (*dirFS)(nil)
	_ fs.ReadFileFS = (*dirFS)(nil)
	_ fs.ReadDirFS  = (*dirFS)(nil)
	_ fs.StatFS     = (*dirFS)(nil)
)
