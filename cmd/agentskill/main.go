// Command agentskill mirrors the skills-ref CLI: validate a skill
// directory, print its frontmatter as JSON, or render the
// available_skills block for one or more skill directories.
//
//	agentskill validate <skill-dir>
//	agentskill read-properties <skill-dir>
//	agentskill to-prompt <skill-dir>...
//
// Each argument may also be the SKILL.md file itself. validate exits 1
// on any error-severity problem and prints warnings without failing;
// the other commands exit 1 when the skill cannot be read.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ChristopherDavenport/agentskill"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage: agentskill <command> [arguments]

commands:
  validate <skill-dir>          validate a skill directory
  read-properties <skill-dir>   print the frontmatter as JSON
  to-prompt <skill-dir>...      render the available_skills block
`

// run executes the command line and returns the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "validate":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		return validate(skillDir(args[1]), stdout, stderr)
	case "read-properties":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		return readProperties(skillDir(args[1]), stdout, stderr)
	case "to-prompt":
		if len(args) < 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		dirs := make([]string, 0, len(args)-1)
		for _, a := range args[1:] {
			dirs = append(dirs, skillDir(a))
		}
		return toPrompt(dirs, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "agentskill: unknown command %q\n%s", args[0], usage)
	return 2
}

// skillDir maps a path to the SKILL.md file onto its directory.
func skillDir(p string) string {
	if info, err := os.Stat(p); err == nil && !info.IsDir() && strings.EqualFold(filepath.Base(p), "SKILL.md") {
		return filepath.Dir(p)
	}
	return p
}

func validate(dir string, stdout, stderr io.Writer) int {
	problems, err := problemsOf(dir)
	if err != nil {
		fmt.Fprintf(stderr, "Validation failed for %s:\n  - %s\n", dir, err)
		return 1
	}
	if agentskill.HasErrors(problems) {
		fmt.Fprintf(stderr, "Validation failed for %s:\n", dir)
		for _, p := range problems {
			if p.Severity == agentskill.Error {
				fmt.Fprintf(stderr, "  - %s\n", p.Message)
			} else {
				fmt.Fprintf(stderr, "  - %s\n", p)
			}
		}
		return 1
	}
	for _, p := range problems {
		fmt.Fprintf(stderr, "  - %s\n", p)
	}
	fmt.Fprintf(stdout, "Valid skill: %s\n", dir)
	return 0
}

// problemsOf loads and validates the directory. The reference
// validator reports a missing directory and a missing SKILL.md as
// validation failures, so those are returned as errors for the caller
// to print in the same frame.
func problemsOf(dir string) ([]agentskill.Problem, error) {
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		//lint:ignore ST1005 mirrors the reference CLI's wording
		return nil, fmt.Errorf("Path does not exist: %s", dir)
	case err != nil:
		return nil, err
	case !info.IsDir():
		//lint:ignore ST1005 mirrors the reference CLI's wording
		return nil, fmt.Errorf("Not a directory: %s", dir)
	}
	s, err := agentskill.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	return s.Validate(), nil
}

// properties is the JSON the reference read-properties prints, keys in
// its order, optional ones omitted when unset.
type properties struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	AllowedTools  string            `json:"allowed-tools,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// load reads the skill the way the reference read_properties does: a
// parse failure or a missing or empty name or description is an error.
// shown is the directory as the reference names it in a not-found
// error, which read-properties prints as given and to-prompt resolved.
func load(dir, shown string) (*agentskill.Skill, error) {
	s, err := agentskill.LoadDir(dir)
	if errors.Is(err, agentskill.ErrNoSkillFile) {
		return nil, fmt.Errorf("SKILL.md not found in %s", shown)
	}
	if err != nil {
		return nil, err
	}
	for _, p := range s.Validate() {
		if p.Field != "name" && p.Field != "description" {
			continue
		}
		if strings.HasPrefix(p.Message, "Missing required field") || strings.Contains(p.Message, "non-empty string") {
			return nil, errors.New(p.Message)
		}
	}
	return s, nil
}

func readProperties(dir string, stdout, stderr io.Writer) int {
	s, err := load(dir, dir)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %s\n", err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(properties{
		Name:          s.Name,
		Description:   s.Description,
		License:       s.License,
		Compatibility: s.Compatibility,
		AllowedTools:  s.AllowedTools,
		Metadata:      s.Metadata,
	}); err != nil {
		fmt.Fprintf(stderr, "Error: %s\n", err)
		return 1
	}
	return 0
}

func toPrompt(dirs []string, stdout, stderr io.Writer) int {
	c := &agentskill.Catalog{}
	for _, dir := range dirs {
		shown := dir
		if abs, err := filepath.Abs(dir); err == nil {
			if resolved, err := filepath.EvalSymlinks(abs); err == nil {
				shown = resolved
			} else {
				shown = abs
			}
		}
		s, err := load(dir, shown)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %s\n", err)
			return 1
		}
		c.Skills = append(c.Skills, s)
	}
	fmt.Fprintln(stdout, c.Prompt())
	return 0
}
