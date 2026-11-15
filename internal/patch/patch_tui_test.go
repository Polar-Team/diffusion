package patch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func newTestModel(t *testing.T) analyzeRoleTUIReader {
	t.Helper()
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{})
	if len(m.plain) == 0 {
		t.Fatal("expected non-empty body lines")
	}
	return m
}

// writeFixtureRoleWithEmptyInclude writes a minimal role with a bare
// "include_tasks:" (no value), which makes the analyzer emit the
// "cannot determine include target" warning and an opaque branch.
func writeFixtureRoleWithEmptyInclude(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
- name: Broken include
  include_tasks:
- name: Ensure thing
  file:
    path: /tmp/thing
    state: touch
`
	if err := os.WriteFile(filepath.Join(tasks, "main.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func update(t *testing.T, m analyzeRoleTUIReader, msg tea.Msg) analyzeRoleTUIReader {
	t.Helper()
	mi, _ := m.Update(msg)
	next, ok := mi.(analyzeRoleTUIReader)
	if !ok {
		t.Fatalf("Update returned unexpected model type %T", mi)
	}
	return next
}

func TestTUICursorMovement(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)

	// Move down.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("after down expected cursor 1, got %d", m.cursor)
	}

	// Move up back to 0, then clamp at top.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Runes: []rune("k"), Type: tea.KeyRunes})
	if m.cursor != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", m.cursor)
	}

	// Jump to bottom, clamp.
	m = update(t, m, tea.KeyMsg{Runes: []rune("G"), Type: tea.KeyRunes})
	if m.cursor != n-1 {
		t.Fatalf("expected cursor at bottom %d, got %d", n-1, m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != n-1 {
		t.Fatalf("cursor should clamp at bottom %d, got %d", n-1, m.cursor)
	}

	// j moves down too; home/end mirror g/G.
	m = update(t, m, tea.KeyMsg{Runes: []rune("g"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("j"), Type: tea.KeyRunes})
	if m.cursor != 1 {
		t.Fatalf("j should move down, got cursor %d", m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if m.cursor != 0 {
		t.Fatalf("home should jump to top, got cursor %d", m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.cursor != n-1 {
		t.Fatalf("end should jump to bottom, got cursor %d", m.cursor)
	}
}

func TestTUIQuit(t *testing.T) {
	m := newTestModel(t)
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyCtrlC},
		{Runes: []rune("q"), Type: tea.KeyRunes},
		{Type: tea.KeyEsc},
	} {
		_, cmd := m.Update(k)
		if cmd == nil {
			t.Fatalf("expected quit cmd for %v", k)
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}
	}
}

func TestTUIWindowSizeAndViewport(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	if n < 5 {
		t.Fatalf("fixture too small for scroll test: %d lines", n)
	}

	// Small window forces scrolling.
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 5})
	if !m.ready {
		t.Fatal("expected ready after WindowSizeMsg")
	}
	if m.bodyHeight() != 3 {
		t.Fatalf("expected body height 3 (5-2), got %d", m.bodyHeight())
	}

	// Jump to bottom; offset should move to keep cursor visible.
	m = update(t, m, tea.KeyMsg{Runes: []rune("G"), Type: tea.KeyRunes})
	if m.cursor != n-1 {
		t.Fatalf("expected cursor at bottom, got %d", m.cursor)
	}
	if m.offset+m.bodyHeight() <= m.cursor {
		t.Fatalf("cursor %d not visible: offset=%d bodyHeight=%d", m.cursor, m.offset, m.bodyHeight())
	}

	view := m.View()
	if !strings.Contains(view, m.plain[m.cursor]) {
		t.Errorf("view missing selected plain line %q", m.plain[m.cursor])
	}
	viewLines := strings.Split(view, "\n")
	lastLine := ansi.Strip(viewLines[len(viewLines)-1])
	wantPos := fmt.Sprintf("%d/%d", m.cursor+1, n)
	if !strings.HasSuffix(lastLine, wantPos) {
		t.Errorf("footer (last line) = %q, want suffix %q", lastLine, wantPos)
	}
	if got := strings.Count(view, "\n") + 1; got != m.height {
		t.Errorf("view has %d lines, want exactly m.height=%d", got, m.height)
	}
}

// TestTUIFooterExactPosition pins the footer's position indicator to the
// exact "cursor/total" string on the last rendered line, so a regression
// that merely contains a stray "/" elsewhere (e.g. in a file path) can't
// make the assertion pass trivially.
func TestTUIFooterExactPosition(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})

	view := m.View()
	lines := strings.Split(view, "\n")
	last := ansi.Strip(lines[len(lines)-1])
	want := fmt.Sprintf("%d/%d", m.cursor+1, n)
	if !strings.HasSuffix(last, want) {
		t.Fatalf("last line = %q, want suffix %q", last, want)
	}
}

// TestTUIFooterPositionVisibleOnNarrowTerminal checks that even on a
// narrow 40-column terminal, the help text is truncated (not the
// position indicator), so "cursor/total" is always fully visible.
func TestTUIFooterPositionVisibleOnNarrowTerminal(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 10})
	m = update(t, m, tea.KeyMsg{Runes: []rune("G"), Type: tea.KeyRunes})

	view := m.View()
	lines := strings.Split(view, "\n")
	last := lines[len(lines)-1]
	want := fmt.Sprintf("%d/%d", n, n)
	if !strings.HasSuffix(ansi.Strip(last), want) {
		t.Fatalf("narrow footer = %q, want suffix %q", last, want)
	}
	if ansi.StringWidth(ansi.Strip(last)) > 40 {
		t.Fatalf("footer line exceeds width 40: %q", last)
	}
}

func TestTUIViewContent(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	m = update(t, m, tea.WindowSizeMsg{Width: 160, Height: 20})

	view := m.View()
	if !strings.Contains(view, "move") || !strings.Contains(view, "quit") {
		t.Errorf("view missing footer help text:\n%s", view)
	}
	if !strings.Contains(view, "role fixture.role") {
		t.Errorf("view missing title:\n%s", view)
	}

	// With NoColor, the selected row uses the "▸ " gutter only (no
	// reverse video), so the view must be strictly ANSI-free.
	if strings.Contains(view, "\x1b[") {
		t.Errorf("unexpected ANSI codes in NoColor view:\n%q", view)
	}
}

// TestTUISelectionHighlightIsUnbroken drives a role with an empty
// "include_tasks:" (exercising the warning/opaque-branch path) and
// asserts that whichever line is selected, the reverse-video highlight
// wraps the entire plain line as one unbroken "\x1b[7m...\x1b[0m" pair
// with no other escape codes inside it -- i.e. the warning text embeds
// no stray ANSI that could end the highlight early.
func TestTUISelectionHighlightIsUnbroken(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRoleWithEmptyInclude(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Warnings) == 0 {
		t.Fatal("expected at least one warning from the empty include_tasks")
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{})
	m = update(t, m, tea.WindowSizeMsg{Width: 200, Height: 30})

	for i := range m.plain {
		mi := m
		mi.cursor = i
		mi.clampCursor()
		view := mi.View()
		const open, closeSeq = "\x1b[7m", "\x1b[0m"
		_, after, found := strings.Cut(view, open)
		if !found {
			t.Fatalf("row %d: no reverse-video start found in view:\n%s", i, view)
		}

		highlighted, _, found := strings.Cut(after, closeSeq)
		if !found {
			t.Fatalf("row %d: no reverse-video end found in view:\n%s", i, view)
		}
		if strings.Contains(highlighted, "\x1b[") {
			t.Errorf("row %d: highlighted segment contains embedded ANSI: %q", i, highlighted)
		}
	}
}

func TestRunTreeTUINilAnalysis(t *testing.T) {
	if err := RunTreeTUI(nil, TreeOptions{}); err == nil {
		t.Fatal("expected error for nil analysis")
	}
}

// TestTUIEmptyBody exercises a role analysis with no tree body lines at
// all is not realistic (PrintTree always emits a header), but we can at
// least drive a model whose body ends up empty by filtering everything
// out, and make sure the footer/view never panics and shows 0/0.
func TestTUIEmptyBody(t *testing.T) {
	m := newTestModel(t)
	m.plain = nil
	m.colored = nil
	m.clampCursor()
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("expected cursor/offset reset to 0, got cursor=%d offset=%d", m.cursor, m.offset)
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	view := m.View()
	if !strings.Contains(view, "0/0") {
		t.Errorf("expected footer to show 0/0, got:\n%s", view)
	}
}

func TestTUIBodyShorterThanWindow(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 1000})
	if m.offset != 0 {
		t.Fatalf("expected offset 0 when body fits entirely, got %d", m.offset)
	}
	m = update(t, m, tea.KeyMsg{Runes: []rune("G"), Type: tea.KeyRunes})
	if m.offset != 0 {
		t.Fatalf("expected offset to stay 0 at bottom when everything fits, got %d", m.offset)
	}
}

func TestTUITinyHeights(t *testing.T) {
	cases := []struct {
		h          int
		wantTitle  bool
		wantFooter bool
	}{
		{0, false, false},
		{1, false, false},
		{2, false, true},
		{3, true, true},
	}
	for _, tc := range cases {
		t.Run("height="+strconv.Itoa(tc.h), func(t *testing.T) {
			m := newTestModel(t)
			m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: tc.h})
			titleRows, bodyRows, footerRows := m.layout()

			if (titleRows > 0) != tc.wantTitle {
				t.Errorf("height=%d: titleRows=%d, want present=%v", tc.h, titleRows, tc.wantTitle)
			}
			if (footerRows > 0) != tc.wantFooter {
				t.Errorf("height=%d: footerRows=%d, want present=%v", tc.h, footerRows, tc.wantFooter)
			}
			if titleRows+bodyRows+footerRows != tc.h {
				t.Errorf("height=%d: rows sum to %d, want %d", tc.h, titleRows+bodyRows+footerRows, tc.h)
			}

			view := m.View()
			lineCount := 0
			if view != "" {
				lineCount = strings.Count(view, "\n") + 1
			}
			if lineCount > tc.h {
				t.Errorf("height=%d: view has %d lines (> height):\n%q", tc.h, lineCount, view)
			}
			if tc.wantTitle && !strings.Contains(view, "role fixture.role") {
				t.Errorf("height=%d: expected title in view:\n%q", tc.h, view)
			}
			if !tc.wantTitle && strings.Contains(view, "role fixture.role") {
				t.Errorf("height=%d: title should be dropped:\n%q", tc.h, view)
			}
			// Must never panic; reaching here is itself a pass.
		})
	}
}

func TestTUIResizeWhileScrolled(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	if n < 10 {
		t.Fatalf("fixture too small: %d lines", n)
	}

	// Scroll to the bottom in a small viewport.
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 5})
	m = update(t, m, tea.KeyMsg{Runes: []rune("G"), Type: tea.KeyRunes})
	if m.cursor != n-1 {
		t.Fatalf("expected cursor at bottom, got %d", m.cursor)
	}

	// Shrink further: cursor must remain visible and offset must stay
	// within bounds.
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 3})
	if m.cursor != n-1 {
		t.Fatalf("cursor moved unexpectedly on resize: %d", m.cursor)
	}
	if m.offset+m.bodyHeight() <= m.cursor || m.offset < 0 {
		t.Fatalf("cursor not visible after shrink: offset=%d bodyHeight=%d cursor=%d", m.offset, m.bodyHeight(), m.cursor)
	}
	maxOffset := n - m.scrollHeight()
	maxOffset = max(maxOffset, 0)

	if m.offset > maxOffset {
		t.Fatalf("offset %d exceeds max %d after shrink", m.offset, maxOffset)
	}

	// Grow back: cursor still visible, offset still clamped.
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 40})
	if m.cursor != n-1 {
		t.Fatalf("cursor moved unexpectedly on grow: %d", m.cursor)
	}
	if m.offset+m.bodyHeight() <= m.cursor || m.offset < 0 {
		t.Fatalf("cursor not visible after grow: offset=%d bodyHeight=%d cursor=%d", m.offset, m.bodyHeight(), m.cursor)
	}
}

func TestTUIHalfPageStepsOnTinyViewport(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	if n < 5 {
		t.Fatalf("fixture too small: %d lines", n)
	}
	// Height 3 -> bodyHeight 1 -> scrollHeight 1; ctrl+d/ctrl+u must still
	// move by at least one line rather than getting stuck (0 step).
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 3})
	if m.scrollHeight() != 1 {
		t.Fatalf("expected scrollHeight 1, got %d", m.scrollHeight())
	}
	start := m.cursor
	m = update(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.cursor != start+1 {
		t.Fatalf("ctrl+d on 1-row viewport should move by 1, start=%d got=%d", start, m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.cursor != start {
		t.Fatalf("ctrl+u on 1-row viewport should move back by 1, got=%d want=%d", m.cursor, start)
	}

	// pgdown/pgup are aliases.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursor != start+1 {
		t.Fatalf("pgdown should behave like ctrl+d, got=%d", m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursor != start {
		t.Fatalf("pgup should behave like ctrl+u, got=%d", m.cursor)
	}
}

func TestTUITruncatesLongLines(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 20, Height: 10})
	view := m.View()
	for line := range strings.SplitSeq(view, "\n") {
		if w := ansi.StringWidth(line); w > 20 {
			t.Errorf("line exceeds width 20 (width=%d): %q", w, line)
		}
	}
}

// --- Horizontal scrolling ---

func TestTUIHorizontalScrollStepAndClamp(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	if m.maxXOffset() == 0 {
		t.Fatal("fixture should have a line wider than the 78-cell content width")
	}

	// l moves right by the step.
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	if m.xOffset != horizontalScrollStep {
		t.Fatalf("after l, xOffset = %d, want %d", m.xOffset, horizontalScrollStep)
	}
	// right is an alias for l.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.xOffset != 2*horizontalScrollStep {
		t.Fatalf("after right, xOffset = %d, want %d", m.xOffset, 2*horizontalScrollStep)
	}

	// h moves back by the step.
	m = update(t, m, tea.KeyMsg{Runes: []rune("h"), Type: tea.KeyRunes})
	if m.xOffset != horizontalScrollStep {
		t.Fatalf("after h, xOffset = %d, want %d", m.xOffset, horizontalScrollStep)
	}
	// left is an alias for h.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.xOffset != 0 {
		t.Fatalf("after left, xOffset = %d, want 0", m.xOffset)
	}
	// h at 0 is a no-op (clamped, not negative).
	m = update(t, m, tea.KeyMsg{Runes: []rune("h"), Type: tea.KeyRunes})
	if m.xOffset != 0 {
		t.Fatalf("h at 0 should stay 0, got %d", m.xOffset)
	}

	// l clamps at maxXOffset however many times it's pressed.
	for range 50 {
		m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	}
	if m.xOffset != m.maxXOffset() {
		t.Fatalf("xOffset = %d, want clamped to maxXOffset %d", m.xOffset, m.maxXOffset())
	}

	// 0 resets to column 0.
	m = update(t, m, tea.KeyMsg{Runes: []rune("0"), Type: tea.KeyRunes})
	if m.xOffset != 0 {
		t.Fatalf("after 0, xOffset = %d, want 0", m.xOffset)
	}
}

func TestTUIHorizontalScrollDollarShowsLineEnd(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})

	// Select the widest line.
	widestIdx := 0
	widest := 0
	for i, l := range m.plain {
		if w := ansi.StringWidth(l); w > widest {
			widest = w
			widestIdx = i
		}
	}
	m.cursor = widestIdx
	m.clampCursor()

	m = update(t, m, tea.KeyMsg{Runes: []rune("$"), Type: tea.KeyRunes})
	wantOffset := ansi.StringWidth(m.plain[m.cursor]) - m.contentWidth()
	wantOffset = max(wantOffset, 0)
	if m.xOffset != wantOffset {
		t.Fatalf("$ xOffset = %d, want %d", m.xOffset, wantOffset)
	}

	// The tail of the selected line's plain text should now be visible in
	// the rendered (selected) row.
	plain := m.plain[m.cursor]
	tail := []rune(plain)
	tail = tail[max(len(tail)-6, 0):]
	view := m.View()
	if !strings.Contains(view, string(tail)) {
		t.Fatalf("view does not contain the tail %q of the selected line:\n%s", string(tail), view)
	}
}

func TestTUIHorizontalScrollNoopWhenEverythingFits(t *testing.T) {
	m := newTestModel(t)
	// A very wide terminal: every line fits, so h/l must be no-ops.
	m = update(t, m, tea.WindowSizeMsg{Width: 1000, Height: 10})
	if m.maxXOffset() != 0 {
		t.Fatalf("expected maxXOffset 0 on a wide terminal, got %d", m.maxXOffset())
	}
	for _, k := range []string{"l", "l", "l"} {
		m = update(t, m, tea.KeyMsg{Runes: []rune(k), Type: tea.KeyRunes})
	}
	if m.xOffset != 0 {
		t.Fatalf("expected xOffset to stay 0 when everything fits, got %d", m.xOffset)
	}
	m = update(t, m, tea.KeyMsg{Runes: []rune("h"), Type: tea.KeyRunes})
	if m.xOffset != 0 {
		t.Fatalf("h should also be a no-op, got %d", m.xOffset)
	}
}

func TestTUIHorizontalScrollLeftEllipsis(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})

	// At offset 0, no row should start its content with a left ellipsis.
	view := m.View()
	for line := range strings.SplitSeq(view, "\n") {
		content := strings.TrimPrefix(strings.TrimPrefix(line, "▸ "), "  ")
		if strings.HasPrefix(ansi.Strip(content), "…") {
			t.Errorf("unexpected leading ellipsis at xOffset=0: %q", line)
		}
	}

	// Scroll right; every non-empty, non-blank-padding row that still has
	// content should now show a leading ellipsis.
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	if m.xOffset == 0 {
		t.Fatal("expected xOffset > 0 after scrolling right")
	}
	view = m.View()
	bodyLines := strings.Split(view, "\n")
	// Drop title (first) and footer (last) lines.
	bodyLines = bodyLines[1 : len(bodyLines)-1]
	foundEllipsis := false
	for _, line := range bodyLines {
		content := strings.TrimPrefix(strings.TrimPrefix(line, "▸ "), "  ")
		stripped := ansi.Strip(content)
		if stripped == "" {
			continue // blank padding row, or a line scrolled fully off
		}
		if strings.HasPrefix(stripped, "…") {
			foundEllipsis = true
		}
	}
	if !foundEllipsis {
		t.Errorf("expected at least one row with a leading ellipsis after scrolling right:\n%s", view)
	}
}

func TestTUIHorizontalScrollRowWidths(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 15})
	for _, xOff := range []int{0, 8, 16, 40, 9999} {
		m.xOffset = xOff
		m.clampXOffset()
		view := m.View()
		for line := range strings.SplitSeq(view, "\n") {
			if w := ansi.StringWidth(line); w > 80 {
				t.Errorf("xOffset=%d: line exceeds width 80 (width=%d): %q", m.xOffset, w, line)
			}
		}
	}
}

func TestTUIHorizontalScrollHighlightStillUnbroken(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 15})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	if m.xOffset == 0 {
		t.Fatal("expected to be scrolled right")
	}

	view := m.View()
	const open, closeSeq = "\x1b[7m", "\x1b[0m"
	_, after, found := strings.Cut(view, open)
	if !found {
		t.Fatalf("no reverse-video start found in view:\n%s", view)
	}

	highlighted, _, found := strings.Cut(after, closeSeq)
	if !found {
		t.Fatalf("no reverse-video end found in view:\n%s", view)
	}
	if strings.Contains(highlighted, "\x1b[") {
		t.Errorf("highlighted segment contains embedded ANSI after scroll: %q", highlighted)
	}
}

func TestTUIHorizontalScrollColorPreserved(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{}) // colors on
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 15})

	// Move the cursor off a long branch line so it renders "colored", not
	// reverse-video, and scroll right.
	m.cursor = 1 // "│   ├── ● 1.1 [copy] ..." (not the cursor row)
	m.clampCursor()
	for range 3 {
		m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	}
	if m.xOffset == 0 {
		t.Fatal("expected to be scrolled right")
	}

	view := m.View()
	lines := strings.Split(view, "\n")
	cursorLineIdx := 1 + (m.cursor - m.offset) // +1 for the title row
	foundColor := false
	for i, line := range lines {
		if i == 0 || i == len(lines)-1 || i == cursorLineIdx {
			continue // skip title, footer, and the (reverse-video) cursor row
		}
		if strings.Contains(line, "\x1b[") {
			foundColor = true
		}
	}
	if !foundColor {
		t.Errorf("expected at least one scrolled, non-selected row to retain its color escape code:\n%s", view)
	}
}

func TestTUIHorizontalScrollNoColorStrictlyAnsiFree(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 15})
	for range 3 {
		m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	}
	if m.xOffset == 0 {
		t.Fatal("expected to be scrolled right")
	}
	view := m.View()
	if strings.Contains(view, "\x1b[") {
		t.Errorf("unexpected ANSI codes in NoColor scrolled view:\n%q", view)
	}
}

func TestTUIHorizontalScrollReclampsOnResize(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	for range 50 {
		m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	}
	wide := m.xOffset
	if wide == 0 {
		t.Fatal("expected to be scrolled to maxXOffset on an 80-col terminal")
	}

	// Growing the terminal reduces (or zeroes) maxXOffset; xOffset must
	// re-clamp instead of staying out of range.
	m = update(t, m, tea.WindowSizeMsg{Width: 1000, Height: 10})
	if m.xOffset > m.maxXOffset() {
		t.Fatalf("xOffset %d exceeds maxXOffset %d after growing", m.xOffset, m.maxXOffset())
	}
	if m.maxXOffset() != 0 {
		t.Fatalf("expected maxXOffset 0 on a 1000-col terminal, got %d", m.maxXOffset())
	}
	if m.xOffset != 0 {
		t.Fatalf("expected xOffset to re-clamp to 0, got %d", m.xOffset)
	}

	// Shrinking back narrows maxXOffset again; xOffset must stay in range.
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 10})
	if m.xOffset > m.maxXOffset() {
		t.Fatalf("xOffset %d exceeds maxXOffset %d after shrinking", m.xOffset, m.maxXOffset())
	}
}

func TestTUIHorizontalScrollFooterShowsColumn(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})

	view := m.View()
	lines := strings.Split(view, "\n")
	last := ansi.Strip(lines[len(lines)-1])
	wantCol := fmt.Sprintf("col %d", m.xOffset)
	if !strings.Contains(last, wantCol) {
		t.Errorf("footer = %q, want it to contain %q", last, wantCol)
	}
	wantPos := fmt.Sprintf("%d/%d", m.cursor+1, n)
	if !strings.HasSuffix(last, wantPos) {
		t.Errorf("footer = %q, want suffix %q", last, wantPos)
	}
}

func TestTUIHorizontalScrollFooterPositionVisibleAtWidth40(t *testing.T) {
	m := newTestModel(t)
	n := len(m.plain)
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 10})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})

	view := m.View()
	lines := strings.Split(view, "\n")
	last := ansi.Strip(lines[len(lines)-1])
	wantPos := fmt.Sprintf("%d/%d", m.cursor+1, n)
	if !strings.HasSuffix(last, wantPos) {
		t.Fatalf("footer at width 40 = %q, want suffix %q", last, wantPos)
	}
	if ansi.StringWidth(last) > 40 {
		t.Fatalf("footer line exceeds width 40: %q", last)
	}
}

func TestTUIVerticalMovesPreserveXOffset(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	want := m.xOffset
	if want == 0 {
		t.Fatal("expected xOffset > 0 before moving the cursor")
	}

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyDown},
		{Type: tea.KeyUp},
		{Runes: []rune("j"), Type: tea.KeyRunes},
		{Runes: []rune("k"), Type: tea.KeyRunes},
		{Runes: []rune("G"), Type: tea.KeyRunes},
		{Runes: []rune("g"), Type: tea.KeyRunes},
	} {
		m = update(t, m, k)
		if m.xOffset != want {
			t.Fatalf("key %v changed xOffset: got %d, want %d", k, m.xOffset, want)
		}
	}
}

// TestTUIScrollHeightFloor guards the scrollHeight >= 1 invariant: at
// heights where the body has 0 rows, cursor/offset arithmetic must still
// use a 1-row window so the cursor can move and stays clamped.
func TestTUIScrollHeightFloor(t *testing.T) {
	for _, h := range []int{0, 1, 2} {
		m := newTestModel(t)
		m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: h})
		if got := m.scrollHeight(); got != 1 {
			t.Fatalf("height %d: scrollHeight = %d, want 1", h, got)
		}
		m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		if m.cursor != 1 {
			t.Fatalf("height %d: j should move cursor to 1, got %d", h, m.cursor)
		}
		if m.offset > m.cursor {
			t.Fatalf("height %d: offset %d past cursor %d", h, m.offset, m.cursor)
		}
	}
}
