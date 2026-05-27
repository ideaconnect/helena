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
