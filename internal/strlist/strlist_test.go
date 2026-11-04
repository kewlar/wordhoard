package strlist_test

import (
	"reflect"
	"testing"

	"wordhoard/internal/strlist"
)

func TestMergeUniqueTrimsDropsBlanksAndDedupesCaseInsensitively(t *testing.T) {
	got := strlist.MergeUnique([]string{" lakoniškas ", "", "Glaustas"}, []string{"glaustas", "trumpas", "  "})
	want := []string{"lakoniškas", "Glaustas", "trumpas"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeUnique = %q, want %q", got, want)
	}
	if got := strlist.MergeUnique(); got == nil || len(got) != 0 {
		t.Errorf("MergeUnique() = %#v, want empty non-nil", got)
	}
}
