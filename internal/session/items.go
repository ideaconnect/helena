package session

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/idct/helena/internal/model"
)

// AddRequest appends a new GET request named name to the container at parentID
// (a collection node like "0" or a folder node like "0/f1") and persists.
// Returns the new request's tree node ID.
func (s *Session) AddRequest(parentID, name string) (string, error) {
	_, _, requestsP := s.containerAtPtr(parentID)
	if requestsP == nil {
		return "", fmt.Errorf("invalid parent: %q", parentID)
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	*requestsP = append(*requestsP, model.Request{
		ID:     model.NewID(),
		Name:   name,
		Method: model.GET,
		Body:   model.Body{Type: model.BodyNone},
	})
	newID := fmt.Sprintf("%s/r%d", parentID, len(*requestsP)-1)
	if err := s.SaveActiveCollection(); err != nil {
		return "", err
	}
	return newID, nil
}

// AddFolder appends a new empty folder named name to the container at parentID
// and persists. Returns the new folder's tree node ID.
func (s *Session) AddFolder(parentID, name string) (string, error) {
	_, foldersP, _ := s.containerAtPtr(parentID)
	if foldersP == nil {
		return "", fmt.Errorf("invalid parent: %q", parentID)
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	*foldersP = append(*foldersP, model.Folder{
		ID:   model.NewID(),
		Name: name,
	})
	newID := fmt.Sprintf("%s/f%d", parentID, len(*foldersP)-1)
	if err := s.SaveActiveCollection(); err != nil {
		return "", err
	}
	return newID, nil
}

// RenameItem renames the collection, folder, or request at nodeID and saves.
// For folder / request renames, also rewrites every peer request's
// ChainStep.Request paths whose prefix matched the old name path so chain
// references survive the rename intact.
func (s *Session) RenameItem(nodeID, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name cannot be empty")
	}
	parent, kind, idx, ok := parseLeaf(nodeID)
	if !ok {
		return fmt.Errorf("invalid node: %q", nodeID)
	}

	// Capture the pre-rename chain-ref path so we can cascade
	// matching ChainStep.Request entries after the rename lands.
	oldPath := s.nodeNamePath(nodeID)

	switch kind {
	case "c":
		if idx < 0 || idx >= len(s.cols) {
			return fmt.Errorf("invalid collection index %d", idx)
		}
		s.cols[idx].Name = name
	case "f":
		_, foldersP, _ := s.containerAtPtr(parent)
		if foldersP == nil || idx < 0 || idx >= len(*foldersP) {
			return fmt.Errorf("invalid folder %q", nodeID)
		}
		(*foldersP)[idx].Name = name
	case "r":
		_, _, requestsP := s.containerAtPtr(parent)
		if requestsP == nil || idx < 0 || idx >= len(*requestsP) {
			return fmt.Errorf("invalid request %q", nodeID)
		}
		(*requestsP)[idx].Name = name
	}

	if (kind == "f" || kind == "r") && oldPath != "" {
		oldParts := strings.Split(oldPath, "/")
		newParts := append([]string(nil), oldParts...)
		newParts[len(newParts)-1] = name
		ci := nodeCollectionIndex(nodeID)
		if ci >= 0 && ci < len(s.cols) {
			cascadeChainRefs(&s.cols[ci], oldParts, newParts)
		}
	}

	return s.SaveActiveCollection()
}

// nodeNamePath returns the chain-ref-style slash-separated display
// name path for nodeID, excluding the collection root. "0/f1/r2" →
// "Auth/Login". Returns "" when nodeID is the bare collection segment
// or can't be resolved against the live tree.
func (s *Session) nodeNamePath(nodeID string) string {
	if nodeID == "" {
		return ""
	}
	segs := strings.Split(nodeID, "/")
	if len(segs) < 2 {
		return ""
	}
	ci, err := strconv.Atoi(segs[0])
	if err != nil || ci < 0 || ci >= len(s.cols) {
		return ""
	}
	folders := s.cols[ci].Folders
	requests := s.cols[ci].Requests
	var parts []string
	for i := 1; i < len(segs); i++ {
		seg := segs[i]
		switch {
		case strings.HasPrefix(seg, "f"):
			fi, err := strconv.Atoi(seg[1:])
			if err != nil || fi < 0 || fi >= len(folders) {
				return ""
			}
			parts = append(parts, folders[fi].Name)
			folders, requests = folders[fi].Folders, folders[fi].Requests
		case strings.HasPrefix(seg, "r"):
			ri, err := strconv.Atoi(seg[1:])
			if err != nil || ri < 0 || ri >= len(requests) {
				return ""
			}
			parts = append(parts, requests[ri].Name)
		}
	}
	return strings.Join(parts, "/")
}

// nodeCollectionIndex extracts the leading collection segment from
// nodeID, returning -1 on malformed input.
func nodeCollectionIndex(nodeID string) int {
	segs := strings.Split(nodeID, "/")
	if len(segs) == 0 {
		return -1
	}
	ci, err := strconv.Atoi(segs[0])
	if err != nil {
		return -1
	}
	return ci
}

// cascadeChainRefs walks every request under col and rewrites any
// ChainStep.Request whose split path begins with oldParts to use
// newParts as the new prefix. The tail (any segments past the prefix)
// is preserved unchanged. Matching is case-sensitive on display names.
func cascadeChainRefs(col *model.Collection, oldParts, newParts []string) {
	rewriteRequests(col.Requests, oldParts, newParts)
	rewriteFoldersChain(col.Folders, oldParts, newParts)
}

func rewriteFoldersChain(folders []model.Folder, oldParts, newParts []string) {
	for i := range folders {
		rewriteRequests(folders[i].Requests, oldParts, newParts)
		rewriteFoldersChain(folders[i].Folders, oldParts, newParts)
	}
}

func rewriteRequests(requests []model.Request, oldParts, newParts []string) {
	for i := range requests {
		for j := range requests[i].Chain {
			ref := requests[i].Chain[j].Request
			if ref == "" {
				continue
			}
			parts := splitChainPath(ref)
			if !hasPrefix(parts, oldParts) {
				continue
			}
			rewrote := append(append([]string(nil), newParts...), parts[len(oldParts):]...)
			requests[i].Chain[j].Request = strings.Join(rewrote, "/")
		}
	}
}

func hasPrefix(s, prefix []string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if s[i] != p {
			return false
		}
	}
	return true
}

// DeleteItem removes the folder or request at nodeID and saves. Collections
// are removed via workspace management, not here.
func (s *Session) DeleteItem(nodeID string) error {
	parent, kind, idx, ok := parseLeaf(nodeID)
	if !ok {
		return fmt.Errorf("invalid node: %q", nodeID)
	}
	switch kind {
	case "c":
		return fmt.Errorf("collections are removed via workspace management")
	case "f":
		_, foldersP, _ := s.containerAtPtr(parent)
		if foldersP == nil || idx < 0 || idx >= len(*foldersP) {
			return fmt.Errorf("invalid folder %q", nodeID)
		}
		*foldersP = slices.Delete(*foldersP, idx, idx+1)
	case "r":
		_, _, requestsP := s.containerAtPtr(parent)
		if requestsP == nil || idx < 0 || idx >= len(*requestsP) {
			return fmt.Errorf("invalid request %q", nodeID)
		}
		*requestsP = slices.Delete(*requestsP, idx, idx+1)
	}
	return s.SaveActiveCollection()
}

// DuplicateItem copies the folder or request at nodeID and inserts the copy
// immediately after the original. Names are suffixed with " (copy)". Returns
// the new node ID.
func (s *Session) DuplicateItem(nodeID string) (string, error) {
	parent, kind, idx, ok := parseLeaf(nodeID)
	if !ok {
		return "", fmt.Errorf("invalid node: %q", nodeID)
	}
	switch kind {
	case "f":
		_, foldersP, _ := s.containerAtPtr(parent)
		if foldersP == nil || idx < 0 || idx >= len(*foldersP) {
			return "", fmt.Errorf("invalid folder %q", nodeID)
		}
		orig := (*foldersP)[idx]
		cp := deepCopyFolder(orig)
		cp.Name = orig.Name + " (copy)"
		*foldersP = slices.Insert(*foldersP, idx+1, cp)
		if err := s.SaveActiveCollection(); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/f%d", parent, idx+1), nil
	case "r":
		_, _, requestsP := s.containerAtPtr(parent)
		if requestsP == nil || idx < 0 || idx >= len(*requestsP) {
			return "", fmt.Errorf("invalid request %q", nodeID)
		}
		orig := (*requestsP)[idx]
		cp := deepCopyRequest(orig)
		cp.Name = orig.Name + " (copy)"
		*requestsP = slices.Insert(*requestsP, idx+1, cp)
		if err := s.SaveActiveCollection(); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/r%d", parent, idx+1), nil
	}
	return "", fmt.Errorf("not duplicable: %s", nodeID)
}

// containerAtPtr returns pointers to the (Folders, Requests) slices of the
// container at id, plus its owning collection. id like "0" addresses a
// collection's root; "0/f1" addresses folder 1 of collection 0; "0/f1/f0"
// addresses a nested folder.
func (s *Session) containerAtPtr(id string) (*model.Collection, *[]model.Folder, *[]model.Request) {
	if id == "" {
		return nil, nil, nil
	}
	parts := strings.Split(id, "/")
	ci, err := strconv.Atoi(parts[0])
	if err != nil || ci < 0 || ci >= len(s.cols) {
		return nil, nil, nil
	}
	col := &s.cols[ci]
	foldersP, requestsP := &col.Folders, &col.Requests
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "f") {
			return nil, nil, nil
		}
		fi, err := strconv.Atoi(p[1:])
		if err != nil || fi < 0 || fi >= len(*foldersP) {
			return nil, nil, nil
		}
		f := &(*foldersP)[fi]
		foldersP, requestsP = &f.Folders, &f.Requests
	}
	return col, foldersP, requestsP
}

// parseLeaf splits a node ID into its parent container, the leaf kind ("c"
// for collection, "f" for folder, "r" for request), and the leaf index.
func parseLeaf(nodeID string) (parent, kind string, idx int, ok bool) {
	if nodeID == "" {
		return "", "", 0, false
	}
	i := strings.LastIndex(nodeID, "/")
	if i < 0 {
		n, err := strconv.Atoi(nodeID)
		if err != nil {
			return "", "", 0, false
		}
		return "", "c", n, true
	}
	parent = nodeID[:i]
	last := nodeID[i+1:]
	if len(last) < 2 {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(last[1:])
	if err != nil {
		return "", "", 0, false
	}
	return parent, string(last[0]), n, true
}

func deepCopyRequest(r model.Request) model.Request {
	r.ID = model.NewID()
	if r.Headers != nil {
		r.Headers = slices.Clone(r.Headers)
	}
	if r.Params != nil {
		r.Params = slices.Clone(r.Params)
	}
	if r.Body.Form != nil {
		r.Body.Form = slices.Clone(r.Body.Form)
	}
	return r
}

func deepCopyFolder(f model.Folder) model.Folder {
	f.ID = model.NewID()
	if f.Requests != nil {
		out := make([]model.Request, len(f.Requests))
		for i, r := range f.Requests {
			out[i] = deepCopyRequest(r)
		}
		f.Requests = out
	}
	if f.Folders != nil {
		out := make([]model.Folder, len(f.Folders))
		for i, sub := range f.Folders {
			out[i] = deepCopyFolder(sub)
		}
		f.Folders = out
	}
	return f
}
