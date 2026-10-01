package agentskill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChristopherDavenport/agenttool"
	"github.com/ChristopherDavenport/openresponses"
)

func fixtureCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := DiscoverDirs("testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func call(t *testing.T, tool agenttool.Tool, args string) (openresponses.Contents, error) {
	t.Helper()
	res, err := tool.Execute(context.Background(), agenttool.Call{ID: "c1", Args: json.RawMessage(args)})
	if err != nil {
		return nil, err
	}
	return res.Output.Parts, nil
}

func TestToolSchema(t *testing.T) {
	tool := fixtureCatalog(t).Tool()
	if tool.Name() != ToolName {
		t.Errorf("Name() = %q", tool.Name())
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "name" {
		t.Errorf("required = %v, want [name]", schema.Required)
	}
	if _, ok := schema.Properties["path"]; !ok {
		t.Error("schema has no path property")
	}
	if agenttool.IsSequential(tool) {
		t.Error("the skill tool must not be sequential")
	}
}

func TestToolBody(t *testing.T) {
	parts, err := call(t, fixtureCatalog(t).Tool(), `{"name":"pdf-processing"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
	text, ok := parts[0].(*openresponses.InputText)
	if !ok {
		t.Fatalf("part is %T, want *InputText", parts[0])
	}
	want := "\n# PDF processing\n\nRead `reference.md` for the API and run `scripts/extract.py` on the file.\n\n1. Extract the text.\n2. Report the tables.\n\nfiles:\n- assets/blob.bin (1024 bytes)\n- assets/logo.png (73 bytes)\n- assets/notes (30 bytes)\n- reference.md (38 bytes)\n- scripts/extract.py (30 bytes)\n"
	if text.Text != want {
		t.Errorf("body =\n%q\nwant\n%q", text.Text, want)
	}

	parts, err = call(t, fixtureCatalog(t).Tool(), `{"name":"minimal"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := parts[0].(*openresponses.InputText).Text; got != "Do the minimal thing.\n\nfiles: none\n" {
		t.Errorf("minimal body = %q", got)
	}
}

func TestToolFiles(t *testing.T) {
	tool := fixtureCatalog(t).Tool()
	tests := []struct {
		name     string
		args     string
		wantText string
		wantImg  string
		wantErr  []string
	}{
		{name: "text by extension", args: `{"name":"pdf-processing","path":"reference.md"}`, wantText: "# Reference\n\nThe pypdf API, in brief.\n"},
		{name: "script", args: `{"name":"pdf-processing","path":"scripts/extract.py"}`, wantText: "import sys\nprint(sys.argv[1])\n"},
		{name: "sniffed text", args: `{"name":"pdf-processing","path":"assets/notes"}`, wantText: "plain notes with no extension\n"},
		{name: "the skill file itself", args: `{"name":"minimal","path":"SKILL.md"}`, wantText: "---\nname: minimal\ndescription: The smallest valid skill.\n---\nDo the minimal thing.\n"},
		{name: "image", args: `{"name":"pdf-processing","path":"assets/logo.png"}`, wantImg: "data:image/png;base64,iVBORw0KGgo"},
		{name: "binary refused", args: `{"name":"pdf-processing","path":"assets/blob.bin"}`, wantErr: []string{`"assets/blob.bin"`, "1024 bytes", "application/octet-stream"}},
		{name: "unknown skill", args: `{"name":"nope"}`, wantErr: []string{`unknown skill "nope"`, "available skills: ", "pdf-processing", "minimal"}},
		{name: "unknown path", args: `{"name":"pdf-processing","path":"missing.md"}`, wantErr: []string{`no file "missing.md"`, "files: assets/blob.bin, assets/logo.png, assets/notes, reference.md, scripts/extract.py"}},
		{name: "directory", args: `{"name":"pdf-processing","path":"scripts"}`, wantErr: []string{`no file "scripts"`}},
		{name: "traversal", args: `{"name":"pdf-processing","path":"../minimal/SKILL.md"}`, wantErr: []string{"invalid path"}},
		{name: "absolute", args: `{"name":"pdf-processing","path":"/etc/passwd"}`, wantErr: []string{"invalid path"}},
		{name: "missing name", args: `{}`, wantErr: []string{"name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := call(t, tool, tt.args)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("got %v, want error", parts)
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q does not contain %q", err, w)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(parts) != 1 {
				t.Fatalf("parts = %d, want 1", len(parts))
			}
			switch p := parts[0].(type) {
			case *openresponses.InputText:
				if tt.wantText == "" || p.Text != tt.wantText {
					t.Errorf("text = %q, want %q", p.Text, tt.wantText)
				}
			case *openresponses.InputImage:
				if tt.wantImg == "" || !strings.HasPrefix(p.ImageURL, tt.wantImg) {
					t.Errorf("image url = %.40q, want prefix %q", p.ImageURL, tt.wantImg)
				}
			default:
				t.Errorf("part is %T", parts[0])
			}
		})
	}
}

// TestToolRecordsWhatItServed: a skill read is the moment a model
// takes instructions from a file on disk, and the only trace of it in
// a session used to be the arguments the model wrote. Every successful
// call carries a Read as Details, with the location behind the name
// and a digest of the bytes served, so a reader can say which SKILL.md
// that was and a replay that serves different bytes is detectable.
func TestToolRecordsWhatItServed(t *testing.T) {
	dir := skillsDir(t)
	tool := fixtureCatalog(t).Tool()
	onDisk := func(parts ...string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(append([]string{dir, "pdf-processing"}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	tests := []struct {
		name     string
		args     string
		wantPath string
		// served is the bytes the digest must cover: the reply itself
		// for the instructions, the file's own bytes for a file.
		served func(res agenttool.Result) []byte
	}{
		{
			name: "instructions",
			args: `{"name":"pdf-processing"}`,
			served: func(res agenttool.Result) []byte {
				return []byte(res.Output.Parts[0].(*openresponses.InputText).Text)
			},
		},
		{
			name:     "text file",
			args:     `{"name":"pdf-processing","path":"scripts/extract.py"}`,
			wantPath: "scripts/extract.py",
			served:   func(agenttool.Result) []byte { return onDisk("scripts", "extract.py") },
		},
		{
			name:     "image, hashed before it is base64 encoded",
			args:     `{"name":"pdf-processing","path":"assets/logo.png"}`,
			wantPath: "assets/logo.png",
			served:   func(agenttool.Result) []byte { return onDisk("assets", "logo.png") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tool.Execute(context.Background(), agenttool.Call{ID: "c1", Args: json.RawMessage(tt.args)})
			if err != nil {
				t.Fatal(err)
			}
			read, ok := res.Details.(Read)
			if !ok {
				t.Fatalf("Details = %#v, want a Read", res.Details)
			}
			served := tt.served(res)
			sum := sha256.Sum256(served)
			want := Read{
				Name:     "pdf-processing",
				Location: dir + "/pdf-processing/SKILL.md",
				Path:     tt.wantPath,
				Bytes:    len(served),
				SHA256:   hex.EncodeToString(sum[:]),
			}
			if read != want {
				t.Errorf("Details = %+v, want %+v", read, want)
			}

			// A recorder that knows nothing about skills can write it.
			rec, err := agenttool.RecordOf(res.Details)
			if err != nil {
				t.Fatal(err)
			}
			if rec == nil {
				t.Fatal("RecordOf() = nil; the details are not Recordable")
			}
			if rec.NS != RecordNS || rec.NS != read.RecordNS() {
				t.Errorf("namespace = %q, want %q", rec.NS, RecordNS)
			}
			var back Read
			if err := json.Unmarshal(rec.Data, &back); err != nil {
				t.Fatal(err)
			}
			if back != want {
				t.Errorf("recorded %+v, want %+v", back, want)
			}
			if tt.wantPath == "" && strings.Contains(string(rec.Data), `"path"`) {
				t.Errorf("recorded %s, want no path for a read of the instructions", rec.Data)
			}
		})
	}
}

// A call that serves nothing records nothing.
func TestToolErrorHasNoRecord(t *testing.T) {
	res, err := fixtureCatalog(t).Tool().Execute(context.Background(), agenttool.Call{ID: "c1", Args: json.RawMessage(`{"name":"nope"}`)})
	if err == nil {
		t.Fatal("want an error")
	}
	if res.Details != nil {
		t.Errorf("Details = %#v, want none", res.Details)
	}
}

func TestToolMaxBytes(t *testing.T) {
	tool := fixtureCatalog(t).Tool(WithMaxBytes(35))
	if _, err := call(t, tool, `{"name":"pdf-processing","path":"scripts/extract.py"}`); err != nil {
		t.Errorf("a 30 byte file under a 35 byte cap: %v", err)
	}
	_, err := call(t, tool, `{"name":"pdf-processing","path":"reference.md"}`)
	if err == nil || !strings.Contains(err.Error(), "38 bytes, over the 35 byte limit") {
		t.Errorf("a 38 byte file over a 35 byte cap: %v", err)
	}
}

func TestDetect(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n rest")
	tests := []struct {
		name string
		data []byte
		kind fileKind
		typ  string
	}{
		{"a.md", []byte("# hi"), kindText, "text/plain"},
		{"Makefile", []byte("all:\n"), kindText, "text/plain"},
		{".gitignore", []byte("x"), kindText, "text/plain"},
		{"a.md", []byte("\xff\xfe bad utf8"), kindBinary, "application/octet-stream"},
		{"a.png", png, kindImage, "image/png"},
		{"a.JPG", []byte("x"), kindImage, "image/jpeg"},
		{"noext", []byte("plain text"), kindText, "text/plain"},
		{"noext", png, kindImage, "image/png"},
		{"noext", []byte("\x00\x01\x02\x03binary"), kindBinary, "application/octet-stream"},
		{"a.pdf", []byte("%PDF-1.4 ..."), kindBinary, "application/pdf"},
		{"a.zip", []byte("PK\x03\x04...."), kindBinary, "application/zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.typ, func(t *testing.T) {
			kind, typ := detect(tt.name, tt.data)
			if kind != tt.kind || typ != tt.typ {
				t.Errorf("detect() = %v, %q; want %v, %q", kind, typ, tt.kind, tt.typ)
			}
		})
	}
}

// TestToolServesTheSkillAsItIsNow: a product whose agent improves its
// own skills edits them after discovery. A read used to serve the body
// from discovery beside the file list from now, a text that never
// existed on disk. The tool reads the skill file at the call: a new
// body is served, and new frontmatter is refused until the product
// discovers again, since the listing and any grant came from the old.
func TestToolServesTheSkillAsItIsNow(t *testing.T) {
	const original = "---\nname: deploy\ndescription: Deploys.\nallowed-tools: bash(kubectl:*)\n---\nStep one: the old way.\n"
	tests := []struct {
		name    string
		edit    func(t *testing.T, dir string)
		want    string
		wantErr string
	}{
		{
			name: "unchanged",
			edit: func(*testing.T, string) {},
			want: "Step one: the old way.\n\nfiles: none\n",
		},
		{
			name: "new body and a new file",
			edit: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "SKILL.md"), strings.Replace(original, "old way", "new way", 1))
				writeFile(t, filepath.Join(dir, "references", "pitfalls.md"), "Not the old way.\n")
			},
			want: "Step one: the new way.\n\nfiles:\n- references/pitfalls.md (17 bytes)\n",
		},
		{
			name: "widened allowed-tools",
			edit: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "SKILL.md"), strings.Replace(original, "bash(kubectl:*)", "bash(*)", 1))
			},
			wantErr: `skill "deploy": the skill file changed since the skills were discovered: the frontmatter of`,
		},
		{
			name: "frontmatter no longer closed",
			edit: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: deploy\n")
			},
			wantErr: "the frontmatter of",
		},
		{
			name: "skill file removed",
			edit: func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "SKILL.md is gone",
		},
		{
			name: "skill file renamed",
			edit: func(t *testing.T, dir string) {
				if err := os.Rename(filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "skill.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "the skill file is now skill.md, not SKILL.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "deploy")
			writeFile(t, filepath.Join(dir, "SKILL.md"), original)
			c, err := DiscoverDirs(root)
			if err != nil {
				t.Fatal(err)
			}
			tt.edit(t, dir)

			res, err := c.Tool().Execute(context.Background(), agenttool.Call{ID: "c1", Args: json.RawMessage(`{"name":"deploy"}`)})
			s, _ := c.Lookup("deploy")
			text, ierr := s.Instructions()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("served %q, want an error", res.Output.Parts[0].(*openresponses.InputText).Text)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				if !errors.Is(ierr, ErrSkillChanged) {
					t.Errorf("Instructions() error = %v, want ErrSkillChanged", ierr)
				}
				if res.Details != nil {
					t.Errorf("Details = %#v, want none", res.Details)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Output.Parts[0].(*openresponses.InputText).Text; got != tt.want {
				t.Errorf("served %q, want %q", got, tt.want)
			}
			if ierr != nil || text != tt.want {
				t.Errorf("Instructions() = %q, %v; want %q", text, ierr, tt.want)
			}
		})
	}
}

// TestInstructionsDigestIsTheRecordedOne: a product binds an approval
// to what the model reads, and the session records Read.SHA256. Both
// must name one digest, computed from Skill.Instructions without
// serving the skill through the tool.
func TestInstructionsDigestIsTheRecordedOne(t *testing.T) {
	c := fixtureCatalog(t)
	tool := c.Tool()
	for _, name := range c.Names() {
		t.Run(name, func(t *testing.T) {
			s, _ := c.Lookup(name)
			text, err := s.Instructions()
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(text))
			args, _ := json.Marshal(toolArgs{Name: name})
			res, err := tool.Execute(context.Background(), agenttool.Call{ID: "c1", Args: args})
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Details.(Read).SHA256; got != hex.EncodeToString(sum[:]) {
				t.Errorf("recorded %s, Instructions hashes to %x", got, sum)
			}
		})
	}
}

// A skill Load did not build serves its Body, with no files.
func TestInstructionsOfAParsedSkill(t *testing.T) {
	s, err := Parse([]byte("---\nname: a\ndescription: d\n---\nBody."))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Instructions()
	if err != nil || got != "Body.\n\nfiles: none\n" {
		t.Errorf("Instructions() = %q, %v", got, err)
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
