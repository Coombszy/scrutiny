package review

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/navigator"
)

// languages maps file extensions to the Language Server language serving
// them. Languages without an installed server answer navigator.ErrNoServer.
var languages = map[string]string{
	".go": "go", ".ts": "typescript", ".tsx": "typescript", ".js": "javascript", ".jsx": "javascript",
	".mjs": "javascript", ".py": "python", ".rs": "rust", ".java": "java", ".rb": "ruby", ".c": "c",
	".h": "c", ".cc": "cpp", ".cpp": "cpp", ".hpp": "cpp", ".cs": "csharp", ".lua": "lua", ".sh": "bash",
	".bash": "bash", ".yaml": "yaml", ".yml": "yaml", ".json": "json", ".tf": "terraform", ".zig": "zig",
}

var (
	// ErrNotAtHead is returned for navigation from a view of another commit
	// than the PR's head: the Language Server only sees the head.
	ErrNotAtHead = errors.New("navigation only works on the pull request's head")
	// ErrBadPath is returned for a path that is not inside the repository.
	ErrBadPath = errors.New("path is not inside the repository")
)

// Language returns the language of a file, or "" if none is known.
func Language(file string) string { return languages[strings.ToLower(path.Ext(file))] }

// Target is where a navigation lands, and whether that file is changed in
// the PR (so it may be shown in the diff).
type Target struct {
	navigator.Location
	InPR bool
}

// Reference is a usage of a symbol, with the text of its line.
type Reference struct {
	Target
	Text string
}

// workspaces tracks the Language Server Workspaces opened for each PR and
// the head their worktree was at.
type workspaces struct {
	mu    sync.Mutex
	heads map[domain.PRRef]domain.SHA
	open  map[domain.PRRef]map[navigator.Workspace]bool
}

func (w *workspaces) init() {
	if w.heads == nil {
		w.heads = map[domain.PRRef]domain.SHA{}
		w.open = map[domain.PRRef]map[navigator.Workspace]bool{}
	}
}

// workspace returns the Workspace for file in the PR's worktree at the
// head last read, moving the worktree and restarting servers when the head
// has moved since they started.
func (s *Service) workspace(ctx context.Context, ref domain.PRRef, head domain.SHA, file string) (navigator.Workspace, snapshot, error) {
	if !validPath(file) {
		return navigator.Workspace{}, snapshot{}, fmt.Errorf("%w: %q", ErrBadPath, file)
	}
	lang := Language(file)
	if lang == "" || s.Navigator == nil {
		return navigator.Workspace{}, snapshot{}, fmt.Errorf("%w for %s", navigator.ErrNoServer, path.Base(file))
	}
	snap, err := s.snapshot(ctx, ref)
	if err != nil {
		return navigator.Workspace{}, snapshot{}, err
	}
	if head != snap.Head {
		return navigator.Workspace{}, snapshot{}, fmt.Errorf("%w (now %s); refresh, or view a Comparison ending there", ErrNotAtHead, snap.Head.Short())
	}
	return s.workspaceFor(ctx, ref, snap, lang)
}

// validPath reports whether p is a clean relative path inside a repository.
func validPath(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

func (s *Service) workspaceFor(ctx context.Context, ref domain.PRRef, snap snapshot, lang string) (navigator.Workspace, snapshot, error) {
	// Held across the worktree move so concurrent requests cannot move it
	// back and forth.
	s.ws.mu.Lock()
	defer s.ws.mu.Unlock()
	dir, err := s.Repos.Worktree(ctx, ref, snap.Head)
	if err != nil {
		return navigator.Workspace{}, snapshot{}, err
	}
	ws := navigator.Workspace{Root: dir, Language: lang}
	s.ws.init()
	if head, ok := s.ws.heads[ref]; ok && head != snap.Head {
		for open := range s.ws.open[ref] {
			s.Navigator.Reset(open)
		}
		clear(s.ws.open[ref])
	}
	s.ws.heads[ref] = snap.Head
	if s.ws.open[ref] == nil {
		s.ws.open[ref] = map[navigator.Workspace]bool{}
	}
	s.ws.open[ref][ws] = true
	return ws, snap, nil
}

// WorkspacePR returns the PR a Workspace belongs to.
func (s *Service) WorkspacePR(ws navigator.Workspace) (domain.PRRef, bool) {
	s.ws.mu.Lock()
	defer s.ws.mu.Unlock()
	for ref, open := range s.ws.open {
		if open[ws] {
			return ref, true
		}
	}
	return domain.PRRef{}, false
}

// Indexing reports whether any of the PR's Language Servers is indexing.
func (s *Service) Indexing(ref domain.PRRef) bool {
	s.ws.mu.Lock()
	defer s.ws.mu.Unlock()
	for ws := range s.ws.open[ref] {
		if s.Navigator.Indexing(ws) {
			return true
		}
	}
	return false
}

// closeWorkspaces stops the PR's servers and removes its worktree.
func (s *Service) closeWorkspaces(ctx context.Context, ref domain.PRRef) error {
	if s.Navigator == nil {
		return s.Repos.RemoveWorktree(ctx, ref)
	}
	s.ws.mu.Lock()
	s.ws.init()
	for ws := range s.ws.open[ref] {
		s.Navigator.Reset(ws)
	}
	delete(s.ws.open, ref)
	delete(s.ws.heads, ref)
	s.ws.mu.Unlock()
	return s.Repos.RemoveWorktree(ctx, ref)
}

// Definition finds where the symbol at pos in file (at the PR's head) is
// defined.
// head is the commit the position was taken from, which must be the PR's.
func (s *Service) Definition(ctx context.Context, ref domain.PRRef, head domain.SHA, file string, pos navigator.Position) ([]Target, error) {
	ws, snap, err := s.workspace(ctx, ref, head, file)
	if err != nil {
		return nil, err
	}
	locs, err := s.Navigator.Definition(ctx, ws, file, pos)
	if err != nil {
		return nil, err
	}
	return s.targets(ctx, ref, snap, locs)
}

// References finds the usages of the symbol at pos in file.
func (s *Service) References(ctx context.Context, ref domain.PRRef, head domain.SHA, file string, pos navigator.Position) ([]Reference, error) {
	ws, snap, err := s.workspace(ctx, ref, head, file)
	if err != nil {
		return nil, err
	}
	locs, err := s.Navigator.References(ctx, ws, file, pos)
	if err != nil {
		return nil, err
	}
	targets, err := s.targets(ctx, ref, snap, locs)
	if err != nil {
		return nil, err
	}
	lines := map[string][]string{}
	refs := make([]Reference, len(targets))
	for i, t := range targets {
		refs[i] = Reference{Target: t}
		if t.OutsideProject {
			continue
		}
		if _, ok := lines[t.Path]; !ok {
			src, _ := s.Repos.Blob(ctx, ref.Repo, snap.Head, t.Path)
			lines[t.Path] = strings.Split(string(src), "\n")
		}
		if l := lines[t.Path]; t.Line >= 1 && t.Line <= len(l) {
			refs[i].Text = strings.TrimSpace(l[t.Line-1])
		}
	}
	return refs, nil
}

func (s *Service) targets(ctx context.Context, ref domain.PRRef, snap snapshot, locs []navigator.Location) ([]Target, error) {
	changed, err := s.prChanges(ctx, ref, snap)
	if err != nil {
		return nil, err
	}
	out := make([]Target, len(locs))
	for i, l := range locs {
		_, inPR := changed[l.Path]
		out[i] = Target{Location: l, InPR: inPR && !l.OutsideProject}
	}
	return out, nil
}

// prChanges are the files the PR changes (merge base to head), by path.
func (s *Service) prChanges(ctx context.Context, ref domain.PRRef, snap snapshot) (map[string]domain.FileChange, error) {
	changes, err := s.Repos.Changes(ctx, ref.Repo, snap.MergeBase, snap.Head)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]domain.FileChange, len(changes))
	for _, c := range changes {
		byPath[c.Path] = c
	}
	return byPath, nil
}

// FileView is a whole file at the PR's head.
type FileView struct {
	Path  string
	Head  domain.SHA
	Lines [][]highlight.Token
	InPR  bool
}

// FileView loads file at the PR's head, for navigating into files the diff
// does not show.
func (s *Service) FileView(ctx context.Context, ref domain.PRRef, file string) (FileView, error) {
	snap, err := s.snapshot(ctx, ref)
	if err != nil {
		return FileView{}, err
	}
	src, err := s.Repos.Blob(ctx, ref.Repo, snap.Head, file)
	if err != nil {
		return FileView{}, err
	}
	changed, err := s.prChanges(ctx, ref, snap)
	if err != nil {
		return FileView{}, err
	}
	_, inPR := changed[file]
	return FileView{Path: file, Head: snap.Head, Lines: s.Highlighter.Highlight(file, src), InPR: inPR}, nil
}

// Marker flags Symbol Search results the PR added or removed.
type Marker string

const (
	MarkerNone    Marker = ""
	MarkerAdded   Marker = "added"
	MarkerRemoved Marker = "removed"
)

// SearchResult is a file or symbol found by Symbol Search.
type SearchResult struct {
	Name   string // the symbol name, or the file's path
	Kind   string // "file" or the symbol kind
	Path   string
	Line   int // zero for files
	InPR   bool
	Marker Marker
	score  int
}

const searchLimit = 50

// SymbolSearch fuzzy-matches query against the repo's files and symbols at
// the PR's head, putting what the PR added or removed first (PR Weighting)
// and marking it. Among the rest, anything else the PR changed wins ties.
func (s *Service) SymbolSearch(ctx context.Context, ref domain.PRRef, query string) ([]SearchResult, error) {
	snap, err := s.snapshot(ctx, ref)
	if err != nil {
		return nil, err
	}
	changed, err := s.prChanges(ctx, ref, snap)
	if err != nil {
		return nil, err
	}
	files, err := s.Repos.ListFiles(ctx, ref.Repo, snap.Head)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	add := func(r SearchResult, target string) {
		if score, ok := fuzzyScore(query, target); ok {
			r.score = score
			results = append(results, r)
		}
	}
	for _, f := range files {
		c, inPR := changed[f]
		r := SearchResult{Name: f, Kind: "file", Path: f, InPR: inPR}
		if c.Status == domain.Added {
			r.Marker = MarkerAdded
		}
		add(r, f)
	}
	for _, c := range changed {
		if c.Status == domain.Deleted {
			add(SearchResult{Name: c.Path, Kind: "file", Path: c.Path, InPR: true, Marker: MarkerRemoved}, c.Path)
		}
	}
	symbols, err := s.prSymbols(ctx, ref, snap, changed, query)
	if err != nil {
		return nil, err
	}
	for _, sym := range symbols {
		add(sym, sym.Name)
	}

	slices.SortFunc(results, func(a, b SearchResult) int {
		return cmp.Or(
			compareBool(a.Marker != MarkerNone, b.Marker != MarkerNone),
			cmp.Compare(b.score, a.score),
			compareBool(a.InPR, b.InPR),
			compareBool(a.Kind != "file", b.Kind != "file"),
			cmp.Compare(a.Name, b.Name),
		)
	})
	if len(results) > searchLimit {
		results = results[:searchLimit]
	}
	return results, nil
}

// prSymbols asks the Language Server of each language the PR touches for
// workspace symbols, marking those declared on lines the PR added.
func (s *Service) prSymbols(ctx context.Context, ref domain.PRRef, snap snapshot, changed map[string]domain.FileChange, query string) ([]SearchResult, error) {
	if s.Navigator == nil || query == "" {
		return nil, nil
	}
	langs := map[string]bool{}
	for p := range changed {
		if l := Language(p); l != "" {
			langs[l] = true
		}
	}
	added := map[string]map[int]bool{} // path -> added line numbers, loaded on demand
	addedLines := func(p string) map[int]bool {
		if lines, ok := added[p]; ok {
			return lines
		}
		lines := map[int]bool{}
		if fd, err := s.Repos.FileDiff(ctx, ref.Repo, snap.MergeBase, snap.Head, changed[p]); err == nil {
			for _, h := range fd.Hunks {
				for _, l := range h.Lines {
					if l.Kind == domain.Add {
						lines[l.NewNo] = true
					}
				}
			}
		}
		added[p] = lines
		return lines
	}
	var out []SearchResult
	for _, lang := range slices.Sorted(maps.Keys(langs)) {
		ws, _, err := s.workspaceFor(ctx, ref, snap, lang)
		if err != nil {
			return nil, err
		}
		syms, err := s.Navigator.Symbols(ctx, ws, query)
		if err != nil {
			continue // e.g. no server for this language
		}
		for _, sym := range syms {
			if sym.OutsideProject {
				continue
			}
			_, inPR := changed[sym.Path]
			r := SearchResult{Name: sym.Name, Kind: sym.Kind, Path: sym.Path, Line: sym.Line, InPR: inPR}
			if inPR && addedLines(sym.Path)[sym.Line] {
				r.Marker = MarkerAdded
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// compareBool orders true before false.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// fuzzyScore matches query's characters in order within target, ignoring
// case. Matches at word starts and runs of consecutive matches score more,
// and shorter targets win ties.
func fuzzyScore(query, target string) (int, bool) {
	if query == "" {
		return 0, true
	}
	q, t := []rune(strings.ToLower(query)), []rune(target)
	lower := []rune(strings.ToLower(target))
	score, qi, prev := 0, 0, -2
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if lower[ti] != q[qi] {
			continue
		}
		score += 10
		if ti == prev+1 {
			score += 50
		}
		if ti == 0 || strings.ContainsRune("/_-. ", t[ti-1]) || (t[ti] >= 'A' && t[ti] <= 'Z' && t[ti-1] >= 'a' && t[ti-1] <= 'z') {
			score += 100
		}
		prev = ti
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	return score*100 - len(t), true
}
