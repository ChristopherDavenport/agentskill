package agentskill

import (
	"fmt"
	"io/fs"
	"strings"
)

// Catalog is the skills found by [Discover]: the winners in source
// order, the duplicates they shadowed, and every problem found on the
// way.
type Catalog struct {
	// Skills holds the skills the prompt lists, in source order and
	// then directory-name order. A skill with problems is still here;
	// a product that wants only valid skills filters on Problems.
	Skills []*Skill
	// Shadowed holds skills whose name an earlier source had already
	// claimed, kept so a product can report them.
	Shadowed []*Skill
	// Problems maps a skill's location to what [Skill.Validate]
	// reported, or to the one error that stopped it loading.
	Problems map[string][]Problem
}

// Discover loads every skill under the sources: each direct child
// directory holding a SKILL.md is one skill. Sources are searched in
// order and the first skill with a given name wins, as PATH resolves a
// command, so a caller lists the most specific source first. A child
// without a SKILL.md is not a skill and is skipped.
//
// A skill that fails to load is not fatal: the error goes into Problems
// under its location and discovery continues. A source whose FS cannot
// be read at its root is an error, because a missing local directory
// should be skipped by the caller on [Dir]'s error, not silently here.
func Discover(sources ...Source) (*Catalog, error) {
	c := &Catalog{Problems: map[string][]Problem{}}
	seen := map[string]bool{}
	for _, src := range sources {
		if src.FS == nil {
			return nil, fmt.Errorf("agentskill: source %q has no FS", src.Location)
		}
		entries, err := fs.ReadDir(src.FS, ".")
		if err != nil {
			return nil, fmt.Errorf("agentskill: read source %q: %w", src.Location, err)
		}
		for _, e := range entries {
			if !isDir(src.FS, e) {
				continue
			}
			file, ok := findSkillFile(src.FS, e.Name())
			if !ok {
				continue
			}
			location := joinLocation(src.Location, e.Name())
			sub, err := fs.Sub(src.FS, e.Name())
			if err != nil {
				c.Problems[joinLocation(location, file)] = []Problem{{Severity: Error, Message: err.Error()}}
				continue
			}
			s, err := Load(sub, location)
			if err != nil {
				c.Problems[joinLocation(location, file)] = []Problem{{Severity: Error, Message: err.Error()}}
				continue
			}
			if problems := s.Validate(); len(problems) > 0 {
				c.Problems[s.Location] = problems
			}
			key := s.Name
			if key == "" {
				key = s.DirName
			}
			if seen[key] {
				c.Shadowed = append(c.Shadowed, s)
				continue
			}
			seen[key] = true
			c.Skills = append(c.Skills, s)
		}
	}
	return c, nil
}

// DiscoverDirs is [Discover] over [Dir] for each path. A path that is
// not a directory is an error.
func DiscoverDirs(dirs ...string) (*Catalog, error) {
	sources := make([]Source, 0, len(dirs))
	for _, dir := range dirs {
		src, err := Dir(dir)
		if err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	return Discover(sources...)
}

// isDir reports whether the entry is a directory, following a symlink
// to one. A link the source refuses, as [Dir] refuses one leading
// outside, is not followed.
func isDir(fsys fs.FS, e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&fs.ModeSymlink == 0 {
		return false
	}
	info, err := fs.Stat(fsys, e.Name())
	return err == nil && info.IsDir()
}

// findSkillFile returns the name of the skill file in the child
// directory, preferring the uppercase spelling.
func findSkillFile(fsys fs.FS, dir string) (string, bool) {
	for _, name := range skillFiles {
		if info, err := fs.Stat(fsys, dir+"/"+name); err == nil && !info.IsDir() {
			return name, true
		}
	}
	return "", false
}

// Listed returns the skills the model is offered, in catalog order:
// the winners that have both a name to call them by and a description
// to choose them from. A skill missing either cannot be used, since
// the name is the only handle the tool takes and the description is
// the only thing the model chooses on, so it is left out of
// [Catalog.Prompt], [Catalog.Names], [Catalog.Lookup] and the tool,
// as the client implementation guide says to skip it and as the
// reference's to_prompt refuses to render it.
//
// It stays in Skills and its problems stay in Problems, so a product
// can still list a broken skill and say what is wrong with it, which
// is why they are loaded at all.
func (c *Catalog) Listed() []*Skill {
	listed := make([]*Skill, 0, len(c.Skills))
	for _, s := range c.Skills {
		if s.Name == "" || s.Description == "" {
			continue
		}
		listed = append(listed, s)
	}
	return listed
}

// Lookup returns the skill named name, among the skills [Catalog.Listed]
// offers, so what the model can be served is what the prompt lists.
func (c *Catalog) Lookup(name string) (*Skill, bool) {
	if name == "" {
		return nil, false
	}
	for _, s := range c.Listed() {
		if s.Name == name {
			return s, true
		}
	}
	return nil, false
}

// Names returns the names of the skills [Catalog.Listed] offers, in
// catalog order.
func (c *Catalog) Names() []string {
	listed := c.Listed()
	names := make([]string, 0, len(listed))
	for _, s := range listed {
		names = append(names, s.Name)
	}
	return names
}

// Prompt renders the available_skills block in the exact shape of the
// reference library's to_prompt: one skill entry per skill in catalog
// order, its name, description and location each on their own line
// between their tags, with the name and description HTML-escaped. For
// local sources the output is byte for byte what skills-ref to-prompt
// prints for the same directories. An empty catalog renders the empty
// block.
//
// Only the skills [Catalog.Listed] offers are rendered: an entry the
// model cannot call, because the skill has no name, or choose, because
// it has no description, would cost tokens on every turn and offer a
// pointer that goes nowhere. The reference refuses to render the whole
// block over such a set; this renders the rest.
func (c *Catalog) Prompt() string {
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range c.Listed() {
		b.WriteString("<skill>\n<name>\n")
		b.WriteString(escape(s.Name))
		b.WriteString("\n</name>\n<description>\n")
		b.WriteString(escape(s.Description))
		b.WriteString("\n</description>\n<location>\n")
		b.WriteString(s.Location)
		b.WriteString("\n</location>\n</skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// Usage is one paragraph telling the model how to use the skills the
// prompt lists through the skill tool. A product appends it to
// [Catalog.Prompt] when it offers [Catalog.Tool].
func (c *Catalog) Usage() string {
	return "When a task matches a skill's description, call the `skill` tool with the skill's name to read its instructions and the list of its files, then call it again with a path to read a file. Follow the instructions before doing the task."
}

// escape is Python's html.escape: the five characters, with the
// apostrophe as &#x27;.
var escape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;").Replace
