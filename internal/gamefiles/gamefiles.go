// Package gamefiles lets admins read and edit the config files a module declares (files: in module.yaml).
//
// The panel never mounts game data. A file is read and written with `docker exec` in the game's container, so the
// exec guard must allowlist the exact commands: ReadArgs and WriteArgs build them, and nothing else ever runs. The
// web side names a file by its id, never by a path.
package gamefiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Spec is one editable file of a game, as the module declares it.
type Spec struct {
	ID    string `yaml:"id" json:"id"`
	Label string `yaml:"label" json:"label"`
	// Path is absolute, inside the container. It may hold * (one path segment each, no ? or []) when the folder
	// has a name only the game knows, such as a world id: it must then match exactly one file, or the editor
	// says how many it found. A file that does not exist yet cannot be created through a pattern.
	Path string `yaml:"path" json:"path"`
	// MaxBytes caps the file (default 65536, at most 1 MiB).
	MaxBytes int `yaml:"max_bytes" json:"max_bytes"`
	// Restart says what a change needs: "required" (the default), "none", or any other text, shown as a hint.
	Restart string `yaml:"restart" json:"restart"`
	// StopFirst warns that the game rewrites this file when it stops, so an edit made while it runs may be lost.
	StopFirst bool `yaml:"stop_first" json:"stop_first"`
	// Redact lists regular expressions; for each line one matches, capture group 1 (the value) is hidden in the editor.
	Redact []string `yaml:"redact" json:"-"`
	// Docs is an https link to documentation about this file, shown beside it in the Files tab.
	Docs string `yaml:"docs" json:"docs,omitempty"`
}

const (
	DefaultMaxBytes = 64 << 10
	AbsoluteMax     = 1 << 20
	// Mask replaces a redacted value in what the editor shows.
	Mask = "••••••••"
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Normalize fills defaults and validates every spec of one module (or game). It returns the list to use.
func Normalize(specs []Spec) ([]Spec, error) {
	out := make([]Spec, len(specs))
	seen := map[string]bool{}
	for i, s := range specs {
		if !idRe.MatchString(s.ID) {
			return nil, fmt.Errorf("files[%d]: id %q must match %s", i, s.ID, idRe)
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("files[%d]: duplicate id %q", i, s.ID)
		}
		seen[s.ID] = true
		if err := checkPath(s.Path); err != nil {
			return nil, fmt.Errorf("files[%s]: %w", s.ID, err)
		}
		if s.Label == "" {
			s.Label = s.ID
		}
		if s.MaxBytes == 0 {
			s.MaxBytes = DefaultMaxBytes
		}
		if s.MaxBytes < 0 || s.MaxBytes > AbsoluteMax {
			return nil, fmt.Errorf("files[%s]: max_bytes must be 1 to %d", s.ID, AbsoluteMax)
		}
		if s.Restart == "" {
			s.Restart = "required"
		}
		if err := checkDocs(s.Docs); err != nil {
			return nil, fmt.Errorf("files[%s]: docs: %w", s.ID, err)
		}
		for _, p := range s.Redact {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, fmt.Errorf("files[%s]: redact %q: %v", s.ID, p, err)
			}
			if re.NumSubexp() < 1 {
				return nil, fmt.Errorf("files[%s]: redact %q needs a capture group around the value to hide", s.ID, p)
			}
		}
		out[i] = s
	}
	return out, nil
}

// checkDocs accepts an empty link or a plain https URL: it is shown as a clickable link to admins, so no other
// scheme, no embedded login, and nothing that is not printable.
func checkDocs(d string) error {
	if d == "" {
		return nil
	}
	if len(d) > 300 {
		return errors.New("must be at most 300 characters")
	}
	for _, r := range d {
		if r <= ' ' || r == 0x7f || r == '"' || r == '\'' || r == '<' || r == '>' || r == '`' {
			return errors.New("has characters that are not allowed in a link")
		}
	}
	u, err := url.Parse(d)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("must be an https:// link")
	}
	return nil
}

func checkPath(p string) error {
	switch {
	case !strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/"):
		return errors.New("path must be an absolute path to a file")
	case strings.ContainsAny(p, "\x00\n\r ?[]{}$`\\\"'") || strings.Contains(p, ".."):
		return errors.New("path has characters that are not allowed (no .., spaces, quotes, or globs other than *)")
	case strings.Contains(path.Ext(p), "*"):
		return errors.New("a * must stay out of the file extension: the syntax check goes by the extension")
	case strings.HasSuffix(p, "*") || strings.HasPrefix(p, "/*"):
		return errors.New("a * must sit inside the path, not at its very start or end, so a pattern cannot reach everything")
	case strings.Contains(p, "//"):
		return errors.New("path must not contain //")
	}
	for _, bad := range []string{"/proc", "/sys", "/dev"} {
		if p == bad || strings.HasPrefix(p, bad+"/") {
			return fmt.Errorf("path under %s is not allowed", bad)
		}
	}
	return nil
}

// IsPattern reports whether Path has a * in it.
func (s Spec) IsPattern() bool { return strings.Contains(s.Path, "*") }

// resolve sets p to the one file the pattern in $1 matches, or stops with exit 3 and says how many it found. The
// pattern comes from the module, never from a request, and has no space or quote in it, so the unquoted $1 only globs.
const resolve = `n=0; for f in $1; do if [ -f "$f" ]; then n=$((n+1)); p=$f; fi; done; [ "$n" -eq 1 ] || { echo "$n files match $1, need exactly 1" >&2; exit 3; }; `

const (
	readPatternScript        = resolve + `head -c "$2" -- "$p"`
	writePatternScript       = resolve + `t="$p.crowsnest-tmp"; cp -p -- "$p" "$t" && cat > "$t" && mv -f -- "$t" "$p"`
	helperWritePatternScript = resolve + `t="$p.crowsnest-tmp"; cp -p -- "$p" "$t" && printf '%s' "$` + ContentEnv + `" > "$t" && mv -f -- "$t" "$p"`
)

// readScript prints the file, or nothing if it does not exist yet (saving then creates it). It reads one byte more
// than the limit so "too large" can be told from "exactly full".
const readScript = `[ -e "$1" ] || exit 0; head -c "$2" -- "$1"`

// ReadArgs is the exact command that prints the file.
func (s Spec) ReadArgs() []string {
	if s.IsPattern() {
		return []string{"sh", "-c", readPatternScript, "sh", s.Path, strconv.Itoa(s.MaxBytes + 1)}
	}
	return []string{"sh", "-c", readScript, "sh", s.Path, strconv.Itoa(s.MaxBytes + 1)}
}

// writeScript copies the old file first so the new one keeps its owner and mode, writes stdin to a temp file beside
// it, then renames it over the original, so a reader never sees half a file.
const writeScript = `t="$1.crowsnest-tmp"; { [ ! -e "$1" ] || cp -p -- "$1" "$t"; } && cat > "$t" && mv -f -- "$t" "$1"`

// WriteArgs is the exact command that replaces the file with its standard input.
func (s Spec) WriteArgs() []string {
	if s.IsPattern() {
		return []string{"sh", "-c", writePatternScript, "sh", s.Path}
	}
	return []string{"sh", "-c", writeScript, "sh", s.Path}
}

// ContentEnv is the environment variable that carries the new content into a helper container (which has no
// standard input to write to).
const ContentEnv = "CROWSNEST_FILE_CONTENT"

// helperWriteScript is writeScript for a one-shot container: the content comes from the environment.
const helperWriteScript = `t="$1.crowsnest-tmp"; { [ ! -e "$1" ] || cp -p -- "$1" "$t"; } && printf '%s' "$` + ContentEnv + `" > "$t" && mv -f -- "$t" "$1"`

// HelperReadArgs and HelperWriteArgs are the entrypoints of the one-shot helper container that edits a file of a
// stopped game. The same fixed scripts as the exec ones, never anything a module supplies.
func (s Spec) HelperReadArgs() []string { return s.ReadArgs() }

// HelperWriteArgs: see HelperReadArgs. The content travels in ContentEnv.
func (s Spec) HelperWriteArgs() []string {
	if s.IsPattern() {
		return []string{"/bin/sh", "-c", helperWritePatternScript, "sh", s.Path}
	}
	return []string{"/bin/sh", "-c", helperWriteScript, "sh", s.Path}
}

// Hash is the sha256 of content, hex.
func Hash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// ErrHiddenValue is returned when a masked line cannot be given its real value back.
var ErrHiddenValue = errors.New("hidden value")

// ErrNotText is returned for a file that is not valid UTF-8 text.
var ErrNotText = errors.New("the file is not UTF-8 text, so it cannot be edited here")

// CheckText refuses binary content.
func CheckText(b []byte) error {
	if !utf8.Valid(b) || strings.IndexByte(string(b), 0) >= 0 {
		return ErrNotText
	}
	return nil
}

// ErrInvalidSyntax is returned when a .json, .yaml, .yml or .xml file would be saved with a syntax error.
var ErrInvalidSyntax = errors.New("invalid syntax")

// CheckSyntax refuses content that cannot be what the file's name says it is. JSON, YAML and XML are checked
// strictly: a game that reads one of these stops at the first typo, so catching it here saves a crash loop. INI files
// get only the mistakes no reader accepts (see checkINI). Other formats (.properties, plain lists) have no strict
// grammar and pass.
func (s Spec) CheckSyntax(content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	switch strings.ToLower(path.Ext(s.Path)) {
	case ".json":
		return checkJSON(content)
	case ".yaml", ".yml":
		return checkYAML(content)
	case ".xml":
		return checkXML(content)
	case ".ini":
		return checkINI(content)
	}
	return nil
}

// checkINI is deliberately loose, since INI has no standard (Unreal's has "+Key=" lines, some readers allow bare keys).
// It refuses a section header that is never closed, a line that starts with "=", and a value that opens a "(" group
// (Unreal style, as in Palworld's OptionSettings) and never closes it. Quoted text is skipped when counting.
func checkINI(content string) error {
	for i, line := range strings.Split(content, "\n") {
		l := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if l == "" || l[0] == ';' || l[0] == '#' {
			continue
		}
		bad := func(msg string) error { return fmt.Errorf("%w in INI: %s (line %d)", ErrInvalidSyntax, msg, i+1) }
		if l[0] == '[' {
			if !strings.Contains(l, "]") {
				return bad("section header is missing its closing ]")
			}
			continue
		}
		if l[0] == '=' {
			return bad("a line cannot start with =")
		}
		_, val, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		if val = strings.TrimSpace(val); strings.HasPrefix(val, "(") {
			depth, quoted := 0, false
			for _, c := range val {
				switch {
				case c == '"':
					quoted = !quoted
				case quoted:
				case c == '(':
					depth++
				case c == ')':
					depth--
				}
			}
			if depth != 0 || quoted {
				return bad("a ( group or quote is never closed")
			}
		}
	}
	return nil
}

func checkJSON(content string) error {
	var v any
	err := json.Unmarshal([]byte(content), &v)
	var se *json.SyntaxError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &se):
		line, col := 1, 1
		for _, c := range []byte(content[:min(int(se.Offset), len(content))]) {
			if c == '\n' {
				line, col = line+1, 1
			} else {
				col++
			}
		}
		return fmt.Errorf("%w in JSON: %s (line %d, column %d)", ErrInvalidSyntax, se.Error(), line, col)
	default:
		return fmt.Errorf("%w in JSON: %s", ErrInvalidSyntax, err.Error())
	}
}

func checkYAML(content string) error {
	dec := yaml.NewDecoder(strings.NewReader(content))
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w in YAML: %s", ErrInvalidSyntax, strings.TrimPrefix(err.Error(), "yaml: "))
		}
	}
}

func checkXML(content string) error {
	dec := xml.NewDecoder(strings.NewReader(content))
	dec.Strict = true
	// Entities such as &nbsp; that are not declared in the file would otherwise fail a file the game accepts.
	dec.Entity = xml.HTMLEntity
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w in XML: %s", ErrInvalidSyntax, err.Error())
		}
	}
}

func (s Spec) regexps() []*regexp.Regexp {
	var res []*regexp.Regexp
	for _, p := range s.Redact {
		if re, err := regexp.Compile(p); err == nil && re.NumSubexp() >= 1 {
			res = append(res, re)
		}
	}
	return res
}

// secretsIn calls fn for every redacted value in line: the text from the start of the match up to the value (which
// names it, like `AdminPassword="`) and the value's position.
func secretsIn(res []*regexp.Regexp, line string, fn func(prefix string, from, to int)) {
	for _, re := range res {
		for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
			if len(m) >= 4 && m[2] >= 0 {
				fn(line[m[0]:m[2]], m[2], m[3])
			}
		}
	}
}

// replaceSecrets rewrites each redacted value of line with what pick returns for its prefix and current text.
func replaceSecrets(res []*regexp.Regexp, line string, pick func(prefix, value string) string) string {
	type span struct {
		from, to int
		with     string
	}
	var spans []span
	secretsIn(res, line, func(prefix string, from, to int) {
		spans = append(spans, span{from, to, pick(prefix, line[from:to])})
	})
	sort.Slice(spans, func(i, j int) bool { return spans[i].from < spans[j].from })
	var out strings.Builder
	last := 0
	for _, sp := range spans {
		if sp.from < last { // two patterns matched the same text
			continue
		}
		out.WriteString(line[last:sp.from])
		out.WriteString(sp.with)
		last = sp.to
	}
	out.WriteString(line[last:])
	return out.String()
}

// Hide returns content as the editor should show it: every redacted value replaced by Mask.
func (s Spec) Hide(content string) string {
	res := s.regexps()
	if len(res) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = replaceSecrets(res, l, func(string, string) string { return Mask })
	}
	return strings.Join(lines, "\n")
}

// Restore undoes Hide on submitted text. A value that still shows Mask gets back the value the same setting (the text
// before it, such as `rcon.password=`) has in current, the file as it is now; where it sits in the file does not
// matter. A mask with no counterpart is an error, so the mask is never written.
func (s Spec) Restore(submitted, current string) (string, error) {
	res := s.regexps()
	if len(res) == 0 {
		return submitted, nil
	}
	secrets := map[string]string{} // prefix -> current value
	for _, l := range strings.Split(current, "\n") {
		secretsIn(res, l, func(prefix string, from, to int) {
			if _, dup := secrets[prefix]; !dup {
				secrets[prefix] = l[from:to]
			}
		})
	}
	lines := strings.Split(submitted, "\n")
	for i, l := range lines {
		var bad error
		lines[i] = replaceSecrets(res, l, func(prefix, value string) string {
			if value != Mask {
				return value
			}
			if v, ok := secrets[prefix]; ok {
				return v
			}
			if bad == nil {
				bad = fmt.Errorf("%w: line %d still shows %s for %q, which is no longer in the file: type the real value or remove it", ErrHiddenValue, i+1, Mask, strings.TrimSpace(prefix))
			}
			return value
		})
		if bad != nil {
			return "", bad
		}
	}
	return strings.Join(lines, "\n"), nil
}
