package session

import (
	"strconv"
	"strings"

	"github.com/idct/helena/internal/model"
)

// Tree presents loaded collections as a navigable tree for widget.Tree.
//
// Node IDs encode a path of indices:
//   - "" is the root; its children are collections "0", "1", …
//   - a collection/folder node's children are folders "…/f<i>" then requests "…/r<i>"
//
// So "0/f1/r0" is the first request of the second folder of the first collection.
type Tree struct {
	cols []model.Collection
}

// ChildIDs returns the child node IDs of id (root when id is "").
func (t *Tree) ChildIDs(id string) []string {
	if id == "" {
		ids := make([]string, len(t.cols))
		for i := range t.cols {
			ids[i] = strconv.Itoa(i)
		}
		return ids
	}
	folders, requests := t.containerAt(id)
	prefix := id + "/"
	ids := make([]string, 0, len(folders)+len(requests))
	for i := range folders {
		ids = append(ids, prefix+"f"+strconv.Itoa(i))
	}
	for i := range requests {
		ids = append(ids, prefix+"r"+strconv.Itoa(i))
	}
	return ids
}

// Search returns the set of node IDs that should stay visible for a sidebar
// filter (#67): every node whose label contains query (case-insensitive),
// each of their ancestors so the path to a match stays reachable, and — when a
// container matches — its whole subtree. Matching spans every loaded
// collection. An empty or whitespace query returns nil, which callers treat as
// "no filter" (show everything).
func (t *Tree) Search(query string) map[string]bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	visible := map[string]bool{}
	var dfs func(id string)
	dfs = func(id string) {
		for _, child := range t.ChildIDs(id) {
			if strings.Contains(strings.ToLower(t.Label(child)), q) {
				t.markMatch(child, visible)
			}
			dfs(child)
		}
	}
	dfs("")
	return visible
}

// markMatch marks id plus its ancestors (so the match is reachable) and, when
// id is a branch, its whole subtree (so a matched folder shows its contents).
func (t *Tree) markMatch(id string, visible map[string]bool) {
	for p := id; p != "" && !visible[p]; {
		visible[p] = true
		if parent := parentID(p); parent != p {
			p = parent
		} else {
			break // reached a collection root; its parent is the virtual root
		}
	}
	t.markSubtree(id, visible)
}

// markSubtree marks every descendant of id visible.
func (t *Tree) markSubtree(id string, visible map[string]bool) {
	if !t.IsBranch(id) {
		return
	}
	for _, child := range t.ChildIDs(id) {
		visible[child] = true
		t.markSubtree(child, visible)
	}
}

// IsBranch reports whether id is a collection or folder (request nodes are leaves).
func (t *Tree) IsBranch(id string) bool {
	if id == "" {
		return true
	}
	return !strings.HasPrefix(lastSegment(id), "r")
}

// Label returns the display text for id.
func (t *Tree) Label(id string) string {
	if id == "" {
		return ""
	}
	if strings.HasPrefix(lastSegment(id), "r") {
		if r, ok := t.Request(id); ok {
			if r.Method != "" {
				return string(r.Method) + "  " + r.Name
			}
			return r.Name
		}
		return ""
	}
	return t.nameAt(id)
}

// Request returns a pointer to the request addressed by id, if it is a request
// node. The pointer indexes the loaded collections, so edits are visible to the
// session until the collections are reloaded.
func (t *Tree) Request(id string) (*model.Request, bool) {
	last := lastSegment(id)
	if !strings.HasPrefix(last, "r") {
		return nil, false
	}
	ri, err := strconv.Atoi(last[1:])
	if err != nil {
		return nil, false
	}
	_, requests := t.containerAt(parentID(id))
	if ri < 0 || ri >= len(requests) {
		return nil, false
	}
	return &requests[ri], true
}

// containerAt returns the (folders, requests) of the collection or folder at id.
func (t *Tree) containerAt(id string) ([]model.Folder, []model.Request) {
	parts := strings.Split(id, "/")
	ci, err := strconv.Atoi(parts[0])
	if err != nil || ci < 0 || ci >= len(t.cols) {
		return nil, nil
	}
	folders, requests := t.cols[ci].Folders, t.cols[ci].Requests
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "f") {
			return nil, nil
		}
		fi, err := strconv.Atoi(p[1:])
		if err != nil || fi < 0 || fi >= len(folders) {
			return nil, nil
		}
		f := folders[fi]
		folders, requests = f.Folders, f.Requests
	}
	return folders, requests
}

func (t *Tree) nameAt(id string) string {
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		ci, err := strconv.Atoi(parts[0])
		if err != nil || ci < 0 || ci >= len(t.cols) {
			return ""
		}
		return t.cols[ci].Name
	}
	folders, _ := t.containerAt(parentID(id))
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "f") {
		if fi, err := strconv.Atoi(last[1:]); err == nil && fi >= 0 && fi < len(folders) {
			return folders[fi].Name
		}
	}
	return ""
}

// AncestorAuths returns the Auth values on every container above id (the
// immediate parent first, the collection root last). Used by callers that
// want to flatten an Inherit on the addressed node via auth.Resolve.
// Returns nil when id is empty or addresses a node outside the tree.
func (t *Tree) AncestorAuths(id string) []model.Auth {
	if id == "" {
		return nil
	}
	var out []model.Auth
	for p := parentID(id); p != id; p = parentID(p) {
		if a, ok := t.containerAuth(p); ok {
			out = append(out, a)
		}
		if p == "" {
			break
		}
		id = p
	}
	return out
}

// containerAuth returns the Auth on the collection or folder addressed by
// id. Returns false for request nodes and out-of-range indices.
func (t *Tree) containerAuth(id string) (model.Auth, bool) {
	if id == "" {
		return model.Auth{}, false
	}
	parts := strings.Split(id, "/")
	ci, err := strconv.Atoi(parts[0])
	if err != nil || ci < 0 || ci >= len(t.cols) {
		return model.Auth{}, false
	}
	col := t.cols[ci]
	if len(parts) == 1 {
		return col.Auth, true
	}
	folders := col.Folders
	var folder *model.Folder
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "f") {
			return model.Auth{}, false
		}
		fi, err := strconv.Atoi(p[1:])
		if err != nil || fi < 0 || fi >= len(folders) {
			return model.Auth{}, false
		}
		folder = &folders[fi]
		folders = folder.Folders
	}
	if folder == nil {
		return model.Auth{}, false
	}
	return folder.Auth, true
}

// AncestorVars returns the merged folder-scoped variables (#81) on every folder
// above id, with nearer (inner) folders overriding outer ones. Collection-level
// variables are a separate scope (see activeCollectionVars), so the collection
// root is not consulted here. Disabled variables are skipped.
func (t *Tree) AncestorVars(id string) map[string]string {
	m := map[string]string{}
	if id == "" {
		return m
	}
	for p := parentID(id); p != id; p = parentID(p) {
		if f, ok := t.containerFolder(p); ok {
			for _, v := range f.Variables {
				if v.Enabled {
					if _, exists := m[v.Key]; !exists {
						m[v.Key] = v.Value // walked inner-first, so the innermost folder wins
					}
				}
			}
		}
		if p == "" {
			break
		}
		id = p
	}
	return m
}

// containerFolder returns the folder addressed by id, or false for the
// collection root, request nodes, and out-of-range indices.
func (t *Tree) containerFolder(id string) (*model.Folder, bool) {
	if id == "" {
		return nil, false
	}
	parts := strings.Split(id, "/")
	ci, err := strconv.Atoi(parts[0])
	if err != nil || ci < 0 || ci >= len(t.cols) {
		return nil, false
	}
	if len(parts) == 1 {
		return nil, false // collection root, not a folder
	}
	folders := t.cols[ci].Folders
	var folder *model.Folder
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "f") {
			return nil, false
		}
		fi, err := strconv.Atoi(p[1:])
		if err != nil || fi < 0 || fi >= len(folders) {
			return nil, false
		}
		folder = &folders[fi]
		folders = folder.Folders
	}
	return folder, folder != nil
}

// CollectionIndex returns the collection index a node ID belongs to, or -1.
func (t *Tree) CollectionIndex(id string) int {
	seg := id
	if i := strings.IndexByte(id, '/'); i >= 0 {
		seg = id[:i]
	}
	n, err := strconv.Atoi(seg)
	if err != nil || n < 0 || n >= len(t.cols) {
		return -1
	}
	return n
}

func lastSegment(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

func parentID(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[:i]
	}
	return id
}
