package session

import (
	"reflect"
	"sort"
	"testing"

	"github.com/idct/helena/internal/model"
)

// searchTree builds a two-collection Tree for filter tests.
func searchTree() *Tree {
	return &Tree{cols: []model.Collection{
		{
			Name:     "Users API",
			Requests: []model.Request{{Method: model.GET, Name: "Ping"}}, // 0/r0
			Folders: []model.Folder{{
				Name: "Admin",
				Requests: []model.Request{
					{Method: model.POST, Name: "Ban user"},     // 0/f0/r0
					{Method: model.GET, Name: "List archived"}, // 0/f0/r1
				},
			}},
		},
		{
			Name:     "Orders",
			Requests: []model.Request{{Method: model.POST, Name: "Create order"}}, // 1/r0
		},
	}}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTreeSearchMatchShowsPath verifies a leaf match reveals the match plus its
// ancestors, and nothing else.
func TestTreeSearchMatchShowsPath(t *testing.T) {
	got := searchTree().Search("ban")
	want := []string{"0", "0/f0", "0/f0/r0"}
	if !reflect.DeepEqual(sortedKeys(got), want) {
		t.Errorf("Search(ban) = %v, want %v", sortedKeys(got), want)
	}
}

// TestTreeSearchContainerMatchShowsSubtree verifies that when a container's
// label matches, its whole subtree becomes visible.
func TestTreeSearchContainerMatchShowsSubtree(t *testing.T) {
	got := searchTree().Search("users api")
	// The collection "Users API" matches → it and everything under it is shown.
	want := []string{"0", "0/f0", "0/f0/r0", "0/f0/r1", "0/r0"}
	if !reflect.DeepEqual(sortedKeys(got), want) {
		t.Errorf("Search(users api) = %v, want %v", sortedKeys(got), want)
	}
}

// TestTreeSearchCrossCollection verifies matching spans collections — a match
// in the second collection is found without the first.
func TestTreeSearchCrossCollection(t *testing.T) {
	got := searchTree().Search("order")
	want := []string{"1", "1/r0"}
	if !reflect.DeepEqual(sortedKeys(got), want) {
		t.Errorf("Search(order) = %v, want %v", sortedKeys(got), want)
	}
}

// TestTreeSearchEmptyAndNoMatch verifies an empty query returns nil (no filter)
// and a non-matching query returns an empty (non-nil) set.
func TestTreeSearchEmptyAndNoMatch(t *testing.T) {
	if got := searchTree().Search("   "); got != nil {
		t.Errorf("Search(blank) = %v, want nil", got)
	}
	if got := searchTree().Search("zzz-nope"); got == nil || len(got) != 0 {
		t.Errorf("Search(no-match) = %v, want empty non-nil", got)
	}
}

// TestTreeSearchCaseInsensitive verifies matching ignores case.
func TestTreeSearchCaseInsensitive(t *testing.T) {
	got := searchTree().Search("BAN")
	if !got["0/f0/r0"] {
		t.Errorf("Search(BAN) missed the Ban user request: %v", sortedKeys(got))
	}
}
