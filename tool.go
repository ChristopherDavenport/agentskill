package agentskill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	maxBytes  int64
	fileClaim func(ctx context.Context, s *Skill, file string) ([]agenttool.FactCall, error)
}

// WithMaxBytes sets the largest file the tool returns; a larger one is
// refused with its size. There is no partial read, because a half file
// is a worse instruction than none. The default is [DefaultMaxBytes].
func WithMaxBytes(n int64) ToolOption {
	return func(o *toolOptions) { o.maxBytes = n }
}

// WithFileClaim has the skill tool make the facts claim
// (agenttool.Factual) for each call, naming the file the call would
// serve, so a product's policy can decide the read before it happens:
// hold it to the rules of the product's own read tool, say, where a
// skills directory is a link into the workspace and a skill's file is
// the workspace's .env.
//
// The claim is the call itself followed by the calls fn returns for
// that file. file is a path inside s.FS: for a read of the skill's
// instructions, by no path or by "SKILL.md" in any case, the skill file
// as [Load] read it, and otherwise the path the call names. The claim
// and the call resolve the skill and the path in one place, so they
// cannot name different files. The list of files that follows the
// instructions is not claimed: it names files and serves none.
//
// A call the tool will refuse without reading anything, an unknown
// skill or a path that is not a relative path inside the skill, claims
// the call itself alone, and fn is not called. So does a read of the
// instructions of a skill Load did not build, which serves the body it
// holds and reads no file. An error from fn is the claim's error, which
// a policy reads as a call it cannot decide.
//
// Without this option, or with a nil fn, the tool makes no claim.
func WithFileClaim(fn func(ctx context.Context, s *Skill, file string) ([]agenttool.FactCall, error)) ToolOption {
	return func(o *toolOptions) { o.fileClaim = fn }
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
	// of the skill's own instructions. A call whose path names the
	// skill file, "SKILL.md" in any case, is a read of the
	// instructions and is recorded with "".
	Path string `json:"path,omitempty"`
	// Bytes is the number of bytes served.
	Bytes int `json:"bytes"`
	// SHA256 is the hex digest of the bytes served: for a read of the
	// instructions the text of the reply, which [Skill.Instructions]
	// returns, and for a file its own bytes, before an image is base64
	// encoded into its part.
	SHA256 string `json:"sha256"`
	// FrontmatterSHA256 is [Skill.FrontmatterSHA256]: the digest of the
	// frontmatter the catalogue was built from, whose allowed-tools a
	// host grants on a read of the instructions. SHA256 says what the
	// model read and this says what it was granted, so an approval
	// bound to both catches a rewrite of either.
	FrontmatterSHA256 string `json:"frontmatter_sha256,omitempty"`
	// FrontmatterChanged is set when the skill file on disk holds other
	// frontmatter than the catalogue was built from. The body served is
	// the one on disk, under the frontmatter as loaded: the listing and
	// any grant stay the old ones until the product discovers again.
	FrontmatterChanged bool `json:"frontmatter_changed,omitempty"`
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
// files. A path naming the skill file itself, "SKILL.md" in any case,
// is served as the instructions, not as a file: models know a skill's
// instructions as SKILL.md and ask for it by path, and a host that
// grants allowed-tools on a read of the instructions must see that read
// as one, while the frontmatter stays out of the reply.
//
// Everything the tool returns is bytes from the source, framed, never
// transformed, so the transcript records exactly what the model read.
// The instructions are read from the skill file at the call, as the
// files are, so a reply never puts a body from discovery beside a file
// list from now. A skill whose frontmatter changed since discovery is
// served its new body under the frontmatter as loaded, and the [Read]
// says so; one whose skill file is gone, renamed or unparseable is
// refused with [ErrSkillChanged].
//
// The tool serves the skills [Catalog.Listed] offers, which are the
// ones [Catalog.Prompt] renders, so the model can fetch everything it
// is shown and nothing it is not.
//
// Every successful call sets a [Read] as the result's Details, naming
// the skill, the SKILL.md behind the name and a digest of the bytes
// served. It implements agenttool.Recordable, so a recorder writes it
// under [RecordNS]; the model never sees it.
//
// Built with [WithFileClaim], the tool claims the file each call would
// serve, so a product's policy can decide the read before it happens.
func (c *Catalog) Tool(opts ...ToolOption) agenttool.Tool {
	o := toolOptions{maxBytes: DefaultMaxBytes}
	for _, opt := range opts {
		opt(&o)
	}
	var topts []agenttool.Option
	if o.fileClaim != nil {
		topts = append(topts, agenttool.WithFacts(func(ctx context.Context, args json.RawMessage) (agenttool.Facts, error) {
			return c.fileFacts(ctx, args, o.fileClaim)
		}))
	}
	return agenttool.New(ToolName,
		"Read a skill's instructions, or one of its files. Call with the skill's name for its instructions and the list of its files; call with a name and a path for a file.",
		func(ctx context.Context, a toolArgs) (agenttool.Result, error) {
			s, p, err := c.target(a)
			if err != nil {
				return agenttool.Result{}, err
			}
			var (
				parts  openresponses.Contents
				served []byte
			)
			changed := false
			if p == "" {
				var text string
				text, changed, err = s.instructions()
				served = []byte(text)
				parts = openresponses.Contents{&openresponses.InputText{Text: text}}
			} else {
				parts, served, err = file(s, p, o.maxBytes)
			}
			if err != nil {
				return agenttool.Result{}, err
			}
			sum := sha256.Sum256(served)
			res := agenttool.Parts(parts...)
			res.Details = Read{
				Name:     s.ListedName(),
				Location: s.Location,
				Path:     p,
				Bytes:    len(served),
				SHA256:   hex.EncodeToString(sum[:]),

				FrontmatterSHA256:  s.FrontmatterSHA256(),
				FrontmatterChanged: changed,
			}
			return res, nil
		}, topts...)
}

// target resolves a call of the skill tool to the skill it names and
// the path it would serve, "" for the skill's instructions. A path
// naming the skill file is the instructions. It is the one reading of
// the arguments, which the call and its facts claim both use.
func (c *Catalog) target(a toolArgs) (*Skill, string, error) {
	s, ok := c.Lookup(a.Name)
	if !ok {
		return nil, "", fmt.Errorf("unknown skill %q; available skills: %s", a.Name, listOrNone(c.Names()))
	}
	if a.Path == "" || isSkillFileName(a.Path) {
		return s, "", nil
	}
	if !fs.ValidPath(a.Path) {
		return nil, "", fmt.Errorf("invalid path %q: must be a relative path inside the skill, as listed", a.Path)
	}
	return s, a.Path, nil
}

// fileFacts is the claim [WithFileClaim] makes for a call with args:
// the call itself, then what fn says of the file the call would serve.
// The arguments are decoded as the call decodes them.
func (c *Catalog) fileFacts(ctx context.Context, args json.RawMessage, fn func(context.Context, *Skill, string) ([]agenttool.FactCall, error)) (agenttool.Facts, error) {
	calls := []agenttool.FactCall{{Args: args}}
	a, err := agenttool.Decode[toolArgs](args)
	if err != nil {
		return agenttool.Facts{Calls: calls}, nil
	}
	s, p, err := c.target(a)
	if err != nil {
		return agenttool.Facts{Calls: calls}, nil
	}
	if p == "" {
		if s.file == "" {
			return agenttool.Facts{Calls: calls}, nil
		}
		p = s.file
	}
	more, err := fn(ctx, s, p)
	if err != nil {
		return agenttool.Facts{}, err
	}
	return agenttool.Facts{Calls: append(calls, more...)}, nil
}

// isSkillFileName reports whether p names the skill file at the root,
// in any case. Case is folded because on a case-insensitive file system
// "Skill.md" opens the same file.
func isSkillFileName(p string) bool {
	return strings.EqualFold(p, "SKILL.md")
}

// ErrSkillChanged is returned by [Skill.Instructions], and so by the
// skill tool, when the skill file is no longer one the skill can be
// served from: it is gone, it is now spelled otherwise, or its
// frontmatter no longer parses, so there is no body to separate from
// it. The product discovers the skills again. Changed frontmatter that
// parses is not this error; see [Skill.Instructions].
var ErrSkillChanged = errors.New("the skill file changed since the skills were discovered")

// Instructions returns the text the skill tool serves for a read of
// the skill's instructions: the Markdown body followed by the list of
// the skill's files with their sizes. [Read.SHA256] for such a read is
// the hex SHA-256 of this text, so a product that binds an approval to
// what the model reads computes the digest the session will record:
//
//	text, err := s.Instructions()
//	sum := sha256.Sum256([]byte(text))
//	digest := hex.EncodeToString(sum[:])
//
// The text does not cover the frontmatter, so it says nothing of what
// allowed-tools grants; bind an approval to [Skill.FrontmatterSHA256]
// as well.
//
// The text is read now, not at discovery: a skill [Load] read has its
// skill file read again, and the body is the body on disk. When the
// frontmatter on disk differs from the one loaded, the body is still
// served: the skill's fields, its listing and any grant made from its
// allowed-tools stay those loaded, which is what was approved, until
// the product discovers again. When the file is gone, renamed or no
// longer parses, Instructions returns an error wrapping
// [ErrSkillChanged]. A skill Load did not build serves [Skill.Body].
//
// The file list carries sizes, so the digest changes when any of the
// skill's files is added, removed or resized: the model reads those
// too.
func (s *Skill) Instructions() (string, error) {
	text, _, err := s.instructions()
	return text, err
}

// FrontmatterSHA256 returns the hex SHA-256 of the skill file's
// frontmatter as [Load] read it, the bytes between the fences: what
// the skill's fields, its allowed-tools among them, were parsed from.
// It is "" for a skill Load did not build. It does not change when the
// file on disk does; a product that approves a skill's grant binds the
// approval to it, and a rewrite of allowed-tools reaches a host only
// through a new discovery, under a new digest.
func (s *Skill) FrontmatterSHA256() string {
	if s.file == "" {
		return ""
	}
	sum := sha256.Sum256(s.front)
	return hex.EncodeToString(sum[:])
}

// instructions is [Skill.Instructions], also reporting whether the
// frontmatter on disk differs from the one loaded.
func (s *Skill) instructions() (string, bool, error) {
	body, changed, err := s.currentBody()
	if err != nil {
		return "", false, err
	}
	files, err := s.Files()
	if err != nil {
		return "", false, err
	}
	var b strings.Builder
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
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
	return b.String(), changed, nil
}

// currentBody reads the skill file again and returns its body, and
// whether its frontmatter differs from the one Load read.
func (s *Skill) currentBody() (string, bool, error) {
	if s.FS == nil || s.file == "" {
		return s.Body, false, nil
	}
	file, src, err := readSkillFile(s.FS)
	if errors.Is(err, ErrNoSkillFile) {
		return "", false, fmt.Errorf("skill %q: %w: %s is gone; the host must discover the skills again", s.ListedName(), ErrSkillChanged, s.Location)
	}
	if err != nil {
		return "", false, err
	}
	if file != s.file {
		return "", false, fmt.Errorf("skill %q: %w: the skill file is now %s, not %s; the host must discover the skills again", s.ListedName(), ErrSkillChanged, file, s.file)
	}
	front, body, err := splitFrontmatter(src)
	if err != nil {
		return "", false, fmt.Errorf("skill %q: %w: %s: %w; the host must discover the skills again", s.ListedName(), ErrSkillChanged, s.Location, err)
	}
	return body, !bytes.Equal(front, s.front), nil
}

// file returns one resource file as text or an image, with the file's
// own bytes beside the part.
//
// name is a valid path, as [Catalog.target] checks.
func file(s *Skill, name string, maxBytes int64) (openresponses.Contents, []byte, error) {
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
	return fmt.Errorf("skill %q has no file %q; files: %s", s.ListedName(), name, listOrNone(files))
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
