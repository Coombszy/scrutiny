package lsp

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Coombszy/scrutiny/internal/navigator"
)

// Only the parts of the LSP types Scrutiny uses.

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspLocation struct {
	URI   string    `json:"uri"`
	Range *lspRange `json:"range,omitempty"`
}

type textDocumentPosition struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type symbolInformation struct {
	Name          string      `json:"name"`
	Kind          int         `json:"kind"`
	ContainerName string      `json:"containerName"`
	Location      lspLocation `json:"location"`
}

type progressParams struct {
	Token json.RawMessage `json:"token"`
	Value struct {
		Kind string `json:"kind"`
	} `json:"value"`
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func uriPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	return filepath.FromSlash(u.Path)
}

// toLocation converts an LSP location, making its path relative to root
// when it lies inside it.
func toLocation(root string, l lspLocation) navigator.Location {
	loc := navigator.Location{Path: uriPath(l.URI)}
	if rel, err := filepath.Rel(root, loc.Path); err == nil && !strings.HasPrefix(rel, "..") {
		loc.Path = filepath.ToSlash(rel)
	} else {
		loc.OutsideProject = true
	}
	if r := l.Range; r != nil {
		loc.Line, loc.Col = r.Start.Line+1, r.Start.Character
		loc.EndCol = r.End.Character
		if r.End.Line != r.Start.Line {
			loc.EndCol = r.Start.Character
		}
	}
	return loc
}

// locations decodes a result that may be null, one Location or several.
func locations(root string, raw json.RawMessage) ([]navigator.Location, error) {
	var many []lspLocation
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] != '[' {
		var one lspLocation
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, err
		}
		many = []lspLocation{one}
	} else if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	out := make([]navigator.Location, 0, len(many))
	for _, l := range many {
		out = append(out, toLocation(root, l))
	}
	return out, nil
}

var symbolKinds = map[int]string{
	1: "file", 2: "module", 3: "namespace", 4: "package", 5: "class", 6: "method", 7: "property",
	8: "field", 9: "constructor", 10: "enum", 11: "interface", 12: "function", 13: "variable",
	14: "constant", 15: "string", 16: "number", 17: "boolean", 18: "array", 19: "object",
	20: "key", 21: "null", 22: "enum member", 23: "struct", 24: "event", 25: "operator", 26: "type parameter",
}

func languageID(language string) string { return language }
