package agentskill

import (
	"context"
	"encoding/json"
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
