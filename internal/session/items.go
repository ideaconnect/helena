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

// AddRequestValue appends a fully-populated request r to the container at
// parentID (a collection node like "0" or a folder node like "0/f1") and
// persists. It is the populated sibling of AddRequest: used when saving a
// scratch tab into a collection, where the request already carries the
// user's edited method/URL/headers/body/scripts/etc. A fresh Request.ID
// is always minted (r.ID is overwritten) so the on-disk request gets a
// stable identity independent of the scratch tab's synthetic one. The
// caller is responsible for making parentID's collection the active one
// first, since persistence writes the active collection. Returns the new
// request's tree node ID.
func (s *Session) AddRequestValue(parentID string, r model.Request) (string, error) {
	_, _, requestsP := s.containerAtPtr(parentID)
	if requestsP == nil {
		return "", fmt.Errorf("invalid parent: %q", parentID)
	}
	if strings.TrimSpace(r.Name) == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	r.ID = model.NewID()
	if r.Method == "" {
		r.Method = model.GET
	}
	*requestsP = append(*requestsP, r)
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

// MoveNode moves the folder or request at srcID into the container at
// dstParentID (a collection root like "0" or a folder like "0/f1") at position
// index, within the SAME collection, and persists. index is interpreted
// against the destination container as the caller currently sees it (before
// the move); MoveNode adjusts for the source's removal when both ends share a
// container. Cross-collection moves and dropping a folder into itself or a
// descendant are rejected. Chain references whose path prefix changed are
// rewritten, mirroring RenameItem. Returns the moved node's new tree node ID.
func (s *Session) MoveNode(srcID, dstParentID string, index int) (string, error) {
	srcParent, kind, srcIdx, ok := parseLeaf(srcID)
	if !ok || (kind != "f" && kind != "r") {
		return "", fmt.Errorf("not movable: %q", srcID)
	}
	ci := nodeCollectionIndex(srcID)
	if ci < 0 || ci >= len(s.cols) {
		return "", fmt.Errorf("invalid node: %q", srcID)
	}
	if ci != nodeCollectionIndex(dstParentID) {
		return "", fmt.Errorf("cross-collection moves are not supported")
	}
	// A folder can't be dropped into itself or its own descendant.
	if dstParentID == srcID || strings.HasPrefix(dstParentID, srcID+"/") {
		return "", fmt.Errorf("cannot move a folder into itself")
	}

	// Capture the destination container's stable identity (a folder model.ID,
	// or "" for the collection root) so we can re-find it after the removal
	// shifts positional node IDs.
	dstFolderID, ok := s.folderIDForContainer(dstParentID, ci)
	if !ok {
		return "", fmt.Errorf("invalid drop target: %q", dstParentID)
	}

	oldPath := s.nodeNamePath(srcID)

	// Pull the moved item out of its source container.
	_, srcFoldersP, srcReqsP := s.containerAtPtr(srcParent)
	var movedFolder model.Folder
	var movedReq model.Request
	var movedModelID string
	switch kind {
	case "f":
		if srcFoldersP == nil || srcIdx < 0 || srcIdx >= len(*srcFoldersP) {
			return "", fmt.Errorf("invalid folder %q", srcID)
		}
		movedFolder = (*srcFoldersP)[srcIdx]
		// A stable, unique ID is needed to relocate the item after the removal
		// reshuffles positional node IDs. Mint one only if missing (legacy
		// collections) — never rewrite an existing ID, since open tabs track
		// requests by it.
		if movedFolder.ID == "" {
			movedFolder.ID = model.NewID()
		}
		movedModelID = movedFolder.ID
		*srcFoldersP = slices.Delete(*srcFoldersP, srcIdx, srcIdx+1)
	case "r":
		if srcReqsP == nil || srcIdx < 0 || srcIdx >= len(*srcReqsP) {
			return "", fmt.Errorf("invalid request %q", srcID)
		}
		movedReq = (*srcReqsP)[srcIdx]
		if movedReq.ID == "" {
			movedReq.ID = model.NewID()
		}
		movedModelID = movedReq.ID
		*srcReqsP = slices.Delete(*srcReqsP, srcIdx, srcIdx+1)
	}

	// Same-container reorder: removing the source shifted everything after it
	// down by one, so a target index past the source decrements by one.
	if srcParent == dstParentID && srcIdx < index {
		index--
	}

	// Re-resolve the destination container (the removal may have invalidated a
	// positional pointer) and insert.
	dstFoldersP, dstReqsP := s.containerByFolderID(ci, dstFolderID)
	if dstFoldersP == nil {
		return "", fmt.Errorf("drop target no longer exists")
	}
	switch kind {
	case "f":
		index = clampIndex(index, len(*dstFoldersP))
		*dstFoldersP = slices.Insert(*dstFoldersP, index, movedFolder)
	case "r":
		index = clampIndex(index, len(*dstReqsP))
		*dstReqsP = slices.Insert(*dstReqsP, index, movedReq)
	}

	newID := s.nodeIDOfModel(ci, kind, movedModelID)

	// Cascade chain refs whose prefix was the moved subtree's old path.
	if oldPath != "" {
		if newPath := s.nodeNamePath(newID); newPath != "" && newPath != oldPath {
			cascadeChainRefs(&s.cols[ci], strings.Split(oldPath, "/"), strings.Split(newPath, "/"))
		}
	}

	if err := s.saveCollection(ci); err != nil {
		return "", err
	}
	return newID, nil
}

func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i > n {
		return n
	}
	return i
}

// folderIDForContainer validates that containerID (within collection ci) is a
// container and returns the model.ID of the folder it names, or "" for the
// collection root. ok is false when containerID isn't a valid container.
func (s *Session) folderIDForContainer(containerID string, ci int) (folderID string, ok bool) {
	if containerID == strconv.Itoa(ci) {
		return "", true // collection root
	}
	parent, kind, idx, parsed := parseLeaf(containerID)
	if !parsed || kind != "f" {
		return "", false
	}
	_, foldersP, _ := s.containerAtPtr(parent)
	if foldersP == nil || idx < 0 || idx >= len(*foldersP) {
		return "", false
	}
	f := &(*foldersP)[idx]
	if f.ID == "" { // backfill so the container survives the removal re-resolve
		f.ID = model.NewID()
	}
	return f.ID, true
}

// containerByFolderID returns the child (Folders, Requests) slice pointers of
// the folder with the given model ID inside collection ci, or the collection
// root's slices when folderID is "". Returns nil pointers if not found.
func (s *Session) containerByFolderID(ci int, folderID string) (*[]model.Folder, *[]model.Request) {
	if ci < 0 || ci >= len(s.cols) {
		return nil, nil
	}
	col := &s.cols[ci]
	if folderID == "" {
		return &col.Folders, &col.Requests
	}
	var walk func(foldersP *[]model.Folder) (*[]model.Folder, *[]model.Request)
	walk = func(foldersP *[]model.Folder) (*[]model.Folder, *[]model.Request) {
		for i := range *foldersP {
			f := &(*foldersP)[i]
			if f.ID == folderID {
				return &f.Folders, &f.Requests
			}
			if a, b := walk(&f.Folders); a != nil {
				return a, b
			}
		}
		return nil, nil
	}
	return walk(&col.Folders)
}

// nodeIDOfModel returns the positional tree node ID of the folder or request
// ("f"/"r") with the given model.ID inside collection ci, or "" if not found.
func (s *Session) nodeIDOfModel(ci int, kind, modelID string) string {
	if ci < 0 || ci >= len(s.cols) {
		return ""
	}
	var walk func(prefix string, folders []model.Folder, requests []model.Request) string
	walk = func(prefix string, folders []model.Folder, requests []model.Request) string {
		if kind == "r" {
			for i := range requests {
				if requests[i].ID == modelID {
					return fmt.Sprintf("%s/r%d", prefix, i)
				}
			}
		}
		for i := range folders {
			id := fmt.Sprintf("%s/f%d", prefix, i)
			if kind == "f" && folders[i].ID == modelID {
				return id
			}
			if got := walk(id, folders[i].Folders, folders[i].Requests); got != "" {
				return got
			}
		}
		return ""
	}
	return walk(strconv.Itoa(ci), s.cols[ci].Folders, s.cols[ci].Requests)
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
