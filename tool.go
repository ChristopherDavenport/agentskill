package agentskill

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/ChristopherDavenport/agenttool"
	"github.com/ChristopherDavenport/openresponses"
)

// ToolName is the name of the tool [Catalog.Tool] returns.
const ToolName = "skill"

// DefaultMaxBytes is the size above which the tool refuses a file.
const DefaultMaxBytes = 1 << 20

// ToolOption configures [Catalog.Tool].
type ToolOption func(*toolOptions)

type toolOptions struct {
	maxBytes int64
}

// WithMaxBytes sets the largest file the tool returns; a larger one is
// refused with its size. There is no partial read, because a half file
// is a worse instruction than none. The default is [DefaultMaxBytes].
func WithMaxBytes(n int64) ToolOption {
	return func(o *toolOptions) { o.maxBytes = n }
}

// toolArgs is the argument object of the skill tool.
type toolArgs struct {
	Name string `json:"name" desc:"The skill to read"`
	Path string `json:"path,omitempty" desc:"A file inside the skill, as listed; omit for the instructions"`
}

// Tool returns an agenttool.Tool named "skill". Called with a name
// alone it returns the skill's body followed by the list of its files,
// so the model learns both what to do and what there is to read.
// Called with a name and a path it returns that file: text as text, an
// image as an image part, anything else refused with its size and
// detected type. An unknown skill is an error listing the names the
// model could have used; an unknown path is an error listing the
// files.
//
// Everything the tool returns is bytes from the source, framed, never
// transformed, so the transcript records exactly what the model read.
func (c *Catalog) Tool(opts ...ToolOption) agenttool.Tool {
	o := toolOptions{maxBytes: DefaultMaxBytes}
	for _, opt := range opts {
		opt(&o)
	}
	return agenttool.New(ToolName,
		"Read a skill's instructions, or one of its files. Call with the skill's name for its instructions and the list of its files; call with a name and a path for a file.",
		func(ctx context.Context, a toolArgs) (openresponses.Contents, error) {
			s, ok := c.Lookup(a.Name)
			if !ok {
				return nil, fmt.Errorf("unknown skill %q; available skills: %s", a.Name, listOrNone(c.Names()))
			}
			if a.Path == "" {
				return body(s)
			}
			return file(s, a.Path, o.maxBytes)
		})
}

// body renders the skill's Markdown followed by its file list.
func body(s *Skill) (openresponses.Contents, error) {
	files, err := s.Files()
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(s.Body)
	if !strings.HasSuffix(s.Body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\nfiles:")
	if len(files) == 0 {
		b.WriteString(" none\n")
	} else {
		b.WriteString("\n")
		for _, f := range files {
			b.WriteString("- ")
			b.WriteString(f)
			if info, err := fs.Stat(s.FS, f); err == nil {
				fmt.Fprintf(&b, " (%d bytes)", info.Size())
			}
			b.WriteString("\n")
		}
	}
	return openresponses.Contents{&openresponses.InputText{Text: b.String()}}, nil
}

// file returns one resource file as text or an image.
func file(s *Skill, name string, maxBytes int64) (openresponses.Contents, error) {
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("invalid path %q: must be a relative path inside the skill, as listed", name)
	}
	info, err := fs.Stat(s.FS, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrOutside) {
			return nil, noSuchFile(s, name)
		}
		return nil, fmt.Errorf("stat %q: %w", name, err)
	}
	if info.IsDir() {
		return nil, noSuchFile(s, name)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("file %q is %d bytes, over the %d byte limit; it is not returned in part", name, info.Size(), maxBytes)
	}
	data, err := fs.ReadFile(s.FS, name)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", name, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file %q is %d bytes, over the %d byte limit; it is not returned in part", name, len(data), maxBytes)
	}
	kind, mediaType := detect(name, data)
	switch kind {
	case kindText:
		return openresponses.Contents{&openresponses.InputText{Text: string(data)}}, nil
	case kindImage:
		url := "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
		return openresponses.Contents{&openresponses.InputImage{ImageURL: url}}, nil
	}
	return nil, fmt.Errorf("file %q is %d bytes of %s, which cannot be shown as text or an image", name, len(data), mediaType)
}

func noSuchFile(s *Skill, name string) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	return fmt.Errorf("skill %q has no file %q; files: %s", s.Name, name, listOrNone(files))
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

type fileKind int

const (
	kindBinary fileKind = iota
	kindText
	kindImage
)

// textExtensions are shown as text without sniffing, when their bytes
// are valid UTF-8. The list is the module's own so the verdict does not
// depend on the host's mime database.
var textExtensions = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".text": true, ".rst": true, ".adoc": true,
	".json": true, ".jsonl": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true,
	".xml": true, ".html": true, ".htm": true, ".svg": true, ".css": true, ".csv": true, ".tsv": true,
	".py": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true, ".bat": true,
	".go": true, ".rs": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true, ".java": true, ".kt": true,
	".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".jsx": true, ".rb": true, ".php": true,
	".sql": true, ".r": true, ".scala": true, ".swift": true, ".lua": true, ".pl": true, ".ex": true, ".exs": true,
	".tex": true, ".bib": true, ".mk": true, ".diff": true, ".patch": true, ".env": true, ".gitignore": true,
}

// imageExtensions map to the media type of the image part.
var imageExtensions = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
}

// imageTypes are the sniffed media types shown as an image part: the
// formats models accept, not everything http.DetectContentType knows.
var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// detect decides how a file is shown: by extension, then by a sniff of
// the first bytes. Text must be valid UTF-8, whatever the extension.
func detect(name string, data []byte) (fileKind, string) {
	ext := strings.ToLower(path.Ext(name))
	if ext == "" && strings.HasPrefix(path.Base(name), ".") {
		ext = strings.ToLower(path.Base(name))
	}
	if mediaType, ok := imageExtensions[ext]; ok {
		return kindImage, mediaType
	}
	if textExtensions[ext] || path.Base(name) == "Makefile" || path.Base(name) == "Dockerfile" {
		if utf8.Valid(data) {
			return kindText, "text/plain"
		}
		return kindBinary, "application/octet-stream"
	}
	sniffed := http.DetectContentType(data)
	mediaType, _, _ := strings.Cut(sniffed, ";")
	mediaType = strings.TrimSpace(mediaType)
	switch {
	case strings.HasPrefix(mediaType, "text/"):
		if utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
			return kindText, mediaType
		}
	case imageTypes[mediaType]:
		return kindImage, mediaType
	}
	return kindBinary, mediaType
}
