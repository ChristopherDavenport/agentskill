package agentskill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

// RecordNS is the namespace of the session entry a recorder writes for
// a skill read, the value [Read.RecordNS] returns.
const RecordNS = "agentskill:read"

// Read is what the skill tool did, set as the Result.Details of every
// successful call. It implements agenttool.Recordable, so a recorder
// that knows nothing about skills writes it beside the call as a
// namespaced JSON entry, and the session can then say which SKILL.md
// was served rather than only which name the model wrote.
//
// A name is not an identity: it is what a PATH-style search across
// several sources resolved to at discovery time. Location says which
// skill won, and SHA256 says what its bytes were, so a replay that
// serves different bytes for the same name is detectable.
type Read struct {
	// Name is the skill's name, as the model wrote it.
	Name string `json:"name"`
	// Location is the skill's Location: the SKILL.md that was read.
	Location string `json:"location"`
	// Path is the file inside the skill that was served, "" for a read
	// of the skill's own instructions.
	Path string `json:"path,omitempty"`
	// Bytes is the number of bytes served.
	Bytes int `json:"bytes"`
	// SHA256 is the hex digest of the bytes served: the text of the
	// reply for a read of the instructions, and the file's own bytes
	// for a file, before an image is base64 encoded into its part.
	SHA256 string `json:"sha256"`
}

// RecordNS returns [RecordNS], the namespace a recorder writes the
// read under.
func (Read) RecordNS() string { return RecordNS }

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
//
// The tool serves the skills [Catalog.Listed] offers, which are the
// ones [Catalog.Prompt] renders, so the model can fetch everything it
// is shown and nothing it is not.
//
// Every successful call sets a [Read] as the result's Details, naming
// the skill, the SKILL.md behind the name and a digest of the bytes
// served. It implements agenttool.Recordable, so a recorder writes it
// under [RecordNS]; the model never sees it.
func (c *Catalog) Tool(opts ...ToolOption) agenttool.Tool {
	o := toolOptions{maxBytes: DefaultMaxBytes}
	for _, opt := range opts {
		opt(&o)
	}
	return agenttool.New(ToolName,
		"Read a skill's instructions, or one of its files. Call with the skill's name for its instructions and the list of its files; call with a name and a path for a file.",
		func(ctx context.Context, a toolArgs) (agenttool.Result, error) {
			s, ok := c.Lookup(a.Name)
			if !ok {
				return agenttool.Result{}, fmt.Errorf("unknown skill %q; available skills: %s", a.Name, listOrNone(c.Names()))
			}
			var (
				parts  openresponses.Contents
				served []byte
				err    error
			)
			if a.Path == "" {
				parts, served, err = body(s)
			} else {
				parts, served, err = file(s, a.Path, o.maxBytes)
			}
			if err != nil {
				return agenttool.Result{}, err
			}
			sum := sha256.Sum256(served)
			res := agenttool.Parts(parts...)
			res.Details = Read{
				Name:     s.Name,
				Location: s.Location,
				Path:     a.Path,
				Bytes:    len(served),
				SHA256:   hex.EncodeToString(sum[:]),
			}
			return res, nil
		})
}

// body renders the skill's Markdown followed by its file list, and
// returns the bytes served beside the part that carries them.
func body(s *Skill) (openresponses.Contents, []byte, error) {
	files, err := s.Files()
	if err != nil {
		return nil, nil, err
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
	text := b.String()
	return openresponses.Contents{&openresponses.InputText{Text: text}}, []byte(text), nil
}

// file returns one resource file as text or an image, with the file's
// own bytes beside the part.
func file(s *Skill, name string, maxBytes int64) (openresponses.Contents, []byte, error) {
	if !fs.ValidPath(name) {
		return nil, nil, fmt.Errorf("invalid path %q: must be a relative path inside the skill, as listed", name)
	}
	info, err := fs.Stat(s.FS, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrOutside) {
			return nil, nil, noSuchFile(s, name)
		}
		return nil, nil, fmt.Errorf("stat %q: %w", name, err)
	}
	if info.IsDir() {
		return nil, nil, noSuchFile(s, name)
	}
	if info.Size() > maxBytes {
		return nil, nil, fmt.Errorf("file %q is %d bytes, over the %d byte limit; it is not returned in part", name, info.Size(), maxBytes)
	}
	data, err := fs.ReadFile(s.FS, name)
	if err != nil {
		return nil, nil, fmt.Errorf("read %q: %w", name, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, nil, fmt.Errorf("file %q is %d bytes, over the %d byte limit; it is not returned in part", name, len(data), maxBytes)
	}
	kind, mediaType := detect(name, data)
	switch kind {
	case kindText:
		return openresponses.Contents{&openresponses.InputText{Text: string(data)}}, data, nil
	case kindImage:
		url := "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
		return openresponses.Contents{&openresponses.InputImage{ImageURL: url}}, data, nil
	}
	return nil, nil, fmt.Errorf("file %q is %d bytes of %s, which cannot be shown as text or an image", name, len(data), mediaType)
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
