package ui

import (
	"testing"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

func TestPairRowsAlignsChanges(t *testing.T) {
	lines := []gitdiff.Line{
		{Kind: gitdiff.Hunk, Text: "@@ -1,4 +1,5 @@"},
		{Kind: gitdiff.Context, OldNum: 1, NewNum: 1, Text: "a"},
		{Kind: gitdiff.Del, OldNum: 2, Text: "old1"},
		{Kind: gitdiff.Del, OldNum: 3, Text: "old2"},
		{Kind: gitdiff.Add, NewNum: 2, Text: "new1"},
		{Kind: gitdiff.Add, NewNum: 3, Text: "new2"},
		{Kind: gitdiff.Add, NewNum: 4, Text: "new3"},
		{Kind: gitdiff.Context, OldNum: 4, NewNum: 5, Text: "z"},
	}
	rows := pairRows(lines)

	// hunk, context, 3 paired change rows (2 del vs 3 add), context.
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].hunk == "" {
		t.Errorf("row 0 should be the hunk header")
	}
	// First change row: old1 on the left, new1 on the right.
	c := rows[2]
	if c.lText != "old1" || !c.lDel || c.rText != "new1" || !c.rAdd {
		t.Errorf("row 2 mispaired: %+v", c)
	}
	// Third change row: no left counterpart (only new3 added).
	extra := rows[4]
	if extra.lText != "" || extra.rText != "new3" || !extra.rAdd {
		t.Errorf("row 4 should be an add with a blank left cell: %+v", extra)
	}
	// Trailing context appears on both sides.
	last := rows[5]
	if last.lText != "z" || last.rText != "z" || last.lDel || last.rAdd {
		t.Errorf("last row should be shared context: %+v", last)
	}
}
