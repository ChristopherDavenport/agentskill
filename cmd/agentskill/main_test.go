package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLI covers the command line's framing; the verdicts themselves
// are the library's and are tested there.
func TestCLI(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantOut    string
		wantErrSub string
	}{
		{name: "no args", wantCode: 2, wantErrSub: "usage:"},
		{name: "unknown", args: []string{"frob"}, wantCode: 2, wantErrSub: `unknown command "frob"`},
		{name: "help", args: []string{"--help"}, wantCode: 0, wantOut: usage},
		{name: "validate ok", args: []string{"validate", "../../testdata/skills/minimal"}, wantCode: 0, wantOut: "Valid skill: ../../testdata/skills/minimal\n"},
		{name: "validate via file", args: []string{"validate", "../../testdata/skills/minimal/SKILL.md"}, wantCode: 0, wantOut: "Valid skill: ../../testdata/skills/minimal\n"},
		{name: "validate bad", args: []string{"validate", "../../testdata/skills/bad-name"}, wantCode: 1,
			wantErrSub: "Validation failed for ../../testdata/skills/bad-name:\n  - Skill name 'Bad_Name' must be lowercase\n"},
		{name: "validate missing", args: []string{"validate", "../../testdata/skills/nope"}, wantCode: 1, wantErrSub: "Path does not exist"},
		{name: "validate file not dir", args: []string{"validate", "main.go"}, wantCode: 1, wantErrSub: "Not a directory: main.go"},
		{name: "validate no skill file", args: []string{"validate", "../../testdata/skills/no-skill-md"}, wantCode: 1, wantErrSub: "Missing required file: SKILL.md"},
		{name: "read-properties", args: []string{"read-properties", "../../testdata/skills/pdf-processing"}, wantCode: 0,
			wantOut: "{\n  \"name\": \"pdf-processing\",\n  \"description\": \"Extract text and tables from PDF files, fill forms, merge documents. Use when a task mentions PDFs or .pdf files.\",\n  \"license\": \"Apache-2.0\",\n  \"compatibility\": \"Requires Python 3.11 and the pypdf package.\",\n  \"allowed-tools\": \"Bash(python:*) Read Write\",\n  \"metadata\": {\n    \"author\": \"example-org\",\n    \"version\": \"1.0\"\n  }\n}\n"},
		{name: "read-properties missing name", args: []string{"read-properties", "../../testdata/skills/missing-description"}, wantCode: 1,
			wantErrSub: "Error: Missing required field in frontmatter: description\n"},
		{name: "read-properties no skill file", args: []string{"read-properties", "../../testdata/skills/no-skill-md"}, wantCode: 1,
			wantErrSub: "Error: SKILL.md not found in ../../testdata/skills/no-skill-md\n"},
		{name: "to-prompt one bad", args: []string{"to-prompt", "../../testdata/skills/minimal", "../../testdata/skills/empty-fields"}, wantCode: 1,
			wantErrSub: "Error: Field 'name' must be a non-empty string\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}
			if tt.wantOut != "" && stdout.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
			if tt.wantErrSub != "" && !strings.Contains(stderr.String(), tt.wantErrSub) {
				t.Errorf("stderr = %q, want containing %q", stderr.String(), tt.wantErrSub)
			}
		})
	}
}

func TestToPrompt(t *testing.T) {
	abs, err := filepath.Abs("../../testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"to-prompt", "../../testdata/skills/minimal", "../../testdata/skills/lowercase-skill-md/skill.md"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	want := "<available_skills>\n<skill>\n<name>\nminimal\n</name>\n<description>\nThe smallest valid skill.\n</description>\n<location>\n" + abs + "/minimal/SKILL.md\n</location>\n</skill>\n" +
		"<skill>\n<name>\nlowercase-skill-md\n</name>\n<description>\nUses the lowercase file name.\n</description>\n<location>\n" + abs + "/lowercase-skill-md/skill.md\n</location>\n</skill>\n</available_skills>\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
}

// TestInterop runs the reference skills-ref CLI over every fixture and
// compares its output and exit status with ours, for validate,
// read-properties and to-prompt. It needs uvx and the network, so it
// runs only under SKILLS_INTEROP=1 (make interop).
//
// Two fixtures differ by design and are compared loosely: invalid-yaml,
// whose message text comes from the YAML parser, and nested, where the
// reference stringifies a nested metadata value and this module
// reports it, as the specification says string to string.
func TestInterop(t *testing.T) {
	if os.Getenv("SKILLS_INTEROP") == "" {
		t.Skip("set SKILLS_INTEROP=1 to run against the skills-ref CLI")
	}
	if _, err := exec.LookPath("uvx"); err != nil {
		t.Skip("uvx not found")
	}
	entries, err := os.ReadDir("../../testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join("../../testdata/skills", e.Name())
		for _, cmd := range []string{"validate", "read-properties", "to-prompt"} {
			t.Run(cmd+"/"+e.Name(), func(t *testing.T) {
				ref := exec.Command("uvx", "--from", "skills-ref", "agentskills", cmd, dir)
				var refOut bytes.Buffer
				ref.Stdout, ref.Stderr = &refOut, &refOut
				refCode := 0
				if err := ref.Run(); err != nil {
					var exit *exec.ExitError
					if !errorsAs(err, &exit) {
						t.Fatalf("skills-ref: %v", err)
					}
					refCode = exit.ExitCode()
				}
				var ours bytes.Buffer
				ourCode := run([]string{cmd, dir}, &ours, &ours)
				switch e.Name() {
				case "invalid-yaml":
					// Same verdict, the parser's own words.
					if ourCode != refCode || !strings.Contains(ours.String(), "Invalid YAML in frontmatter") {
						t.Errorf("exit = %d, skills-ref %d\nours:\n%s\nskills-ref:\n%s", ourCode, refCode, ours.String(), refOut.String())
					}
					return
				case "nested":
					// The reference accepts a nested metadata value by
					// stringifying it; this module reports it.
					if cmd == "validate" && (ourCode != 1 || !strings.Contains(ours.String(), "must be a string")) {
						t.Errorf("exit = %d\nours:\n%s", ourCode, ours.String())
					}
					return
				}
				if ourCode != refCode {
					t.Errorf("exit = %d, skills-ref %d\nours:\n%s\nskills-ref:\n%s", ourCode, refCode, ours.String(), refOut.String())
				}
				if ours.String() != refOut.String() {
					t.Errorf("output differs\nours:\n%s\nskills-ref:\n%s", ours.String(), refOut.String())
				}
			})
		}
	}
}

func errorsAs(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}
