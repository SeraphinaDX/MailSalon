package uiapp

import (
	"image"
	"strings"
	"testing"

	rw "github.com/mattn/go-runewidth"
	"github.com/metaspartan/gotui/v5/widgets"
)

func TestWrapComposeLinePreservesTextAndWidth(t *testing.T) {
	original := "This is a fairly long sentence that should wrap at word boundaries."
	segments := wrapComposeLine(original, 18)
	if len(segments) < 2 {
		t.Fatalf("wrapComposeLine returned %d segment(s), want multiple", len(segments))
	}
	if got := strings.Join(segments, ""); got != original {
		t.Fatalf("wrapped text changed: got %q, want %q", got, original)
	}
	for i, segment := range segments {
		if width := rw.StringWidth(segment); width > 18 {
			t.Fatalf("segment %d width = %d, want <= 18: %q", i, width, segment)
		}
	}
}

func TestWrapComposeLineHandlesWideRunes(t *testing.T) {
	original := "界界界界界"
	segments := wrapComposeLine(original, 4)
	if got := strings.Join(segments, ""); got != original {
		t.Fatalf("wrapped wide text changed: got %q, want %q", got, original)
	}
	for i, segment := range segments {
		if width := rw.StringWidth(segment); width > 4 {
			t.Fatalf("wide segment %d width = %d, want <= 4: %q", i, width, segment)
		}
	}
}

func TestWrapComposeBodyLineKeepsCursorVisible(t *testing.T) {
	ta := widgets.NewTextArea()
	ta.SetRect(0, 0, 16, 8)
	original := "alpha beta gamma delta epsilon"
	ta.Text = original
	ta.Cursor = image.Pt(len([]rune(original)), 0)

	wrapComposeBodyLine(ta)

	if !strings.Contains(ta.Text, "\n") {
		t.Fatalf("body was not wrapped: %q", ta.Text)
	}
	if got := strings.ReplaceAll(ta.Text, "\n", ""); got != original {
		t.Fatalf("body text changed while wrapping: got %q, want %q", got, original)
	}
	if ta.Cursor.Y == 0 {
		t.Fatalf("cursor stayed on first logical line after wrap: %+v", ta.Cursor)
	}
	if ta.Cursor.X >= ta.Inner.Dx() {
		t.Fatalf("cursor X = %d, inner width = %d; cursor would be clipped", ta.Cursor.X, ta.Inner.Dx())
	}
}
