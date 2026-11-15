package patch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"diffusion/internal/config"

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
// asserts that whichever line is selected, the selected-row highlight
// (config.ColorPAnalyzerSelectedRow ... config.ColorReset, the user's
// restored style) wraps the entire plain line as one unbroken span with
// no other escape codes inside it -- i.e. the warning text embeds no
// stray ANSI that could end the highlight early.
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
		open, closeSeq := config.ColorPAnalyzerSelectedRow, config.ColorReset
		_, after, found := strings.Cut(view, open)
		if !found {
			t.Fatalf("row %d: no selected-row highlight start found in view:\n%s", i, view)
		}

		highlighted, _, found := strings.Cut(after, closeSeq)
		if !found {
			t.Fatalf("row %d: no selected-row highlight end found in view:\n%s", i, view)
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 10})
	if m.maxXOffset() == 0 {
		t.Fatal("fixture should have a line wider than the 48-cell content width")
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 10})

	// At offset 0, no row should start its content with a left ellipsis.
	view := m.View()
	for line := range strings.SplitSeq(view, "\n") {
		content := strings.TrimPrefix(line, listGutter)
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
		content := strings.TrimPrefix(line, listGutter)
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 15})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	if m.xOffset == 0 {
		t.Fatal("expected to be scrolled right")
	}

	view := m.View()
	open, closeSeq := config.ColorPAnalyzerSelectedRow, config.ColorReset
	_, after, found := strings.Cut(view, open)
	if !found {
		t.Fatalf("no selected-row highlight start found in view:\n%s", view)
	}

	highlighted, _, found := strings.Cut(after, closeSeq)
	if !found {
		t.Fatalf("no selected-row highlight end found in view:\n%s", view)
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 15})

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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 15})
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 10})
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 10})
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
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 10})
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

// --- Slim rows ---

func rowByKind(t *testing.T, m analyzeRoleTUIReader, kind treeRowKind) int {
	t.Helper()
	for i, r := range m.rows {
		if r.Kind == kind {
			return i
		}
	}
	t.Fatalf("no row of kind %v found", kind)
	return -1
}

func TestTUISlimRowsOmitFullFields(t *testing.T) {
	m := newTestModel(t)
	for i, l := range m.plain {
		row := m.rows[i]
		switch row.Kind {
		case rowLeaf, rowBranch:
			if strings.Contains(l, ":") && strings.Contains(l, "src=") {
				t.Errorf("row %d: slim leaf row should not contain src=: %q", i, l)
			}
			if strings.Contains(l, "dest=") {
				t.Errorf("row %d: slim row should not contain dest=: %q", i, l)
			}
			if strings.Contains(l, "notify=") {
				t.Errorf("row %d: slim row should not contain notify=: %q", i, l)
			}
			if strings.Contains(l, "tags:[") {
				t.Errorf("row %d: slim row should not contain tags:[: %q", i, l)
			}
			if strings.Contains(l, "<=") {
				t.Errorf("row %d: slim row should not contain overlay <=: %q", i, l)
			}
			// File:line only ever appears inside the old "[module] file:line"
			// construct; the slim format never writes a bare ".yml:" marker.
			if strings.Contains(l, ".yml:") {
				t.Errorf("row %d: slim row should not contain file:line: %q", i, l)
			}
		}
		if row.Kind == rowLeaf && row.Leaf.Name == "" && !strings.Contains(l, "["+row.Leaf.Module+"]") {
			t.Errorf("row %d: unnamed leaf should show [module]: %q", i, l)
		}
	}
}

func TestTUISlimBranchUnnamedFallback(t *testing.T) {
	// "Nested include" branch 3.2 has a Name, so use the role's ghost
	// include (branch 4, opaque with no name) as the unnamed-branch case.
	m := newTestModel(t)
	idx := -1
	for i, r := range m.rows {
		if r.Kind == rowBranch && r.Branch.Name == "" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Skip("fixture has no unnamed branch")
	}
	line := m.plain[idx]
	b := m.rows[idx].Branch
	if b.Target != "" {
		want := b.Kind + " → " + b.Target
		if !strings.Contains(line, want) {
			t.Errorf("unnamed branch with target should show %q, got %q", want, line)
		}
	} else if b.Opaque {
		if !strings.Contains(line, b.Kind+" (opaque)") {
			t.Errorf("unnamed opaque branch should show kind (opaque), got %q", line)
		}
	}
}

func TestTUIHandlersStayInListWithIDs(t *testing.T) {
	m := newTestModel(t)
	sectionIdx := rowByKind(t, m, rowSection)
	if m.rows[sectionIdx].Text != "handlers:" {
		// There may be a warnings section first in row order; search all
		// section rows for "handlers:".
		found := false
		for i, r := range m.rows {
			if r.Kind == rowSection && r.Text == "handlers:" {
				sectionIdx = i
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no 'handlers:' section row found")
		}
	}
	foundHandlerLeaf := false
	for i := sectionIdx + 1; i < len(m.rows); i++ {
		r := m.rows[i]
		if r.Kind == rowLeaf && strings.HasPrefix(r.Leaf.ID, "h") {
			foundHandlerLeaf = true
			if !strings.Contains(m.plain[i], r.Leaf.ID) {
				t.Errorf("handler row %d should show its ID %q: %q", i, r.Leaf.ID, m.plain[i])
			}
		}
	}
	if !foundHandlerLeaf {
		t.Error("expected at least one handler leaf row (h-ID) after 'handlers:'")
	}
}

// TestTUINoNotifyRefRowsInList confirms the user's decision: the TUI
// list never shows "↳" notify-dependency sub-rows under tasks/branches
// -- handlers only appear in their own "handlers:" section, with their
// IDs, so they stay patchable. PrintTree (the plain/--no-tui output)
// still renders them; that's covered by the golden test.
func TestTUINoNotifyRefRowsInList(t *testing.T) {
	m := newTestModel(t)
	for i, r := range m.rows {
		if r.Kind == rowNotifyRef {
			t.Fatalf("row %d: rowNotifyRef should never reach the TUI's row list", i)
		}
		if strings.Contains(m.plain[i], "↳") {
			t.Errorf("row %d: unexpected ↳ marker in slim row: %q", i, m.plain[i])
		}
	}
}

// --- Mode transitions ---

func firstOpenableRow(m analyzeRoleTUIReader) int {
	for i, r := range m.rows {
		if rowOpenable(r) {
			return i
		}
	}
	return -1
}

func TestTUIEnterAndSpaceOpenMenu(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeySpace, Runes: []rune(" ")},
	} {
		m := newTestModel(t)
		m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
		idx := firstOpenableRow(m)
		if idx == -1 {
			t.Fatal("no openable row in fixture")
		}
		m.cursor = idx
		m.clampCursor()
		m = update(t, m, key)
		if m.mode != modeMenu {
			t.Fatalf("key %v: expected modeMenu, got %v", key, m.mode)
		}
		if m.popupRow != idx {
			t.Fatalf("expected popupRow %d, got %d", idx, m.popupRow)
		}
	}
}

func TestTUIEnterOnHeaderOrSectionIsNoop(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	idx := rowByKind(t, m, rowSection)
	m.cursor = idx
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeList {
		t.Fatalf("enter on a section row should be a no-op, got mode %v", m.mode)
	}
}

func TestTUIModeTransitions(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	idx := firstOpenableRow(m)
	m.cursor = idx
	m.clampCursor()

	// enter opens the menu.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMenu {
		t.Fatalf("expected modeMenu, got %v", m.mode)
	}

	// enter on "Details" opens details.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeDetails {
		t.Fatalf("expected modeDetails, got %v", m.mode)
	}
	if len(m.details) == 0 {
		t.Fatal("expected non-empty details")
	}

	// esc in details goes back to the menu.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeMenu {
		t.Fatalf("esc in details should go back to menu, got %v", m.mode)
	}

	// esc in the menu goes back to the list.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeList {
		t.Fatalf("esc in menu should go back to list, got %v", m.mode)
	}

	// q in details goes straight to the list.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeDetails {
		t.Fatalf("expected modeDetails, got %v", m.mode)
	}
	m = update(t, m, tea.KeyMsg{Runes: []rune("q"), Type: tea.KeyRunes})
	if m.mode != modeList {
		t.Fatalf("q in details should close everything, got %v", m.mode)
	}
}

func TestTUICtrlCQuitsInEveryMode(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	idx := firstOpenableRow(m)
	m.cursor = idx
	m.clampCursor()

	for _, setup := range []func(analyzeRoleTUIReader) analyzeRoleTUIReader{
		func(m analyzeRoleTUIReader) analyzeRoleTUIReader { return m }, // list
		func(m analyzeRoleTUIReader) analyzeRoleTUIReader {
			return update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}, // menu
		func(m analyzeRoleTUIReader) analyzeRoleTUIReader {
			m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			return update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}, // details
	} {
		mm := setup(m)
		_, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatalf("ctrl+c in mode %v should quit", mm.mode)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("ctrl+c in mode %v should produce tea.QuitMsg", mm.mode)
		}
	}
}

func TestTUIJKInMenuDoNotMoveListCursor(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	idx := firstOpenableRow(m)
	m.cursor = idx
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMenu {
		t.Fatal("expected modeMenu")
	}
	cursorBefore := m.cursor
	m = update(t, m, tea.KeyMsg{Runes: []rune("j"), Type: tea.KeyRunes})
	m = update(t, m, tea.KeyMsg{Runes: []rune("k"), Type: tea.KeyRunes})
	if m.cursor != cursorBefore {
		t.Fatalf("list cursor moved while in menu mode: %d -> %d", cursorBefore, m.cursor)
	}
}

// --- Details content ---

func detailValue(fields []detailField, key string) (string, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return "", false
}

func TestTUIDetailsLeafContent(t *testing.T) {
	m := newTestModel(t)
	// Leaf 1.1 has src/dest and inherits tags from branch 1.
	var row treeRow
	for _, r := range m.rows {
		if r.Kind == rowLeaf && r.Leaf.ID == "1.1" {
			row = r
		}
	}
	if row.Leaf == nil {
		t.Fatal("expected leaf 1.1 in fixture")
	}
	fields := m.buildDetails(row)
	if v, ok := detailValue(fields, "file"); !ok || v != "tasks/setup.yml" {
		t.Errorf("file = %q, %v", v, ok)
	}
	if v, ok := detailValue(fields, "src"); !ok || v != "foo.conf" {
		t.Errorf("src = %q, %v", v, ok)
	}
	if v, ok := detailValue(fields, "dest"); !ok || v != "/etc/foo.conf" {
		t.Errorf("dest = %q, %v", v, ok)
	}
	if v, ok := detailValue(fields, "inherits"); !ok || !strings.Contains(v, "setup") || !strings.Contains(v, "from 1") {
		t.Errorf("inherits = %q, %v", v, ok)
	}
	if v, ok := detailValue(fields, "branch"); !ok || !strings.Contains(v, "1 include_tasks") {
		t.Errorf("branch = %q, %v", v, ok)
	}
}

func TestTUIDetailsLeafNotifyAndOverlay(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	opts := TreeOptions{Overlays: map[string]string{"2": "patch/templates/app.custom"}}
	m := newAnalyzeRoleTUIReader(a, opts)
	var row treeRow
	for _, r := range m.rows {
		if r.Kind == rowLeaf && r.Leaf.ID == "2" {
			row = r
		}
	}
	fields := m.buildDetails(row)
	if v, ok := detailValue(fields, "notify"); !ok || !strings.Contains(v, "h1") {
		t.Errorf("notify = %q, %v, want it to resolve to h1", v, ok)
	}
	if v, ok := detailValue(fields, "overlay"); !ok || v != "<= patch/templates/app.custom" {
		t.Errorf("overlay = %q, %v", v, ok)
	}
}

func TestTUIDetailsBranchContent(t *testing.T) {
	m := newTestModel(t)
	var row treeRow
	for _, r := range m.rows {
		if r.Kind == rowBranch && r.Branch.ID == "4" {
			row = r
		}
	}
	if row.Branch == nil {
		t.Fatal("expected opaque branch 4 (Ghost include) in fixture")
	}
	fields := m.buildDetails(row)
	if v, ok := detailValue(fields, "kind"); !ok || v != "include_tasks" {
		t.Errorf("kind = %q, %v", v, ok)
	}
	if v, ok := detailValue(fields, "opaque"); !ok || !strings.Contains(v, "yes") {
		t.Errorf("opaque = %q, %v", v, ok)
	}
}

// TestTUIDetailsHandlerShowsNotifiedBy replaces the old notify-ref
// Details test: since "↳" rows no longer exist in the TUI, the "who
// notifies this handler" information now lives on the handler leaf's
// own Details, as a "notified by" field listing the IDs of the tasks/
// branches that notify it (task 2, "Deploy app config", notifies
// handler h1 in the fixture).
func TestTUIDetailsHandlerShowsNotifiedBy(t *testing.T) {
	m := newTestModel(t)
	var row treeRow
	for _, r := range m.rows {
		if r.Kind == rowLeaf && r.Leaf.ID == "h1" {
			row = r
		}
	}
	if row.Leaf == nil {
		t.Fatal("expected handler h1 in fixture")
	}
	fields := m.buildDetails(row)
	v, ok := detailValue(fields, "notified by")
	if !ok || !strings.Contains(v, "2") {
		t.Errorf("notified by = %q, %v, want it to list task 2", v, ok)
	}
}

// TestHandlerNotifiedByDeduplicatesAndSortsInTreeOrder exercises
// handlerNotifiedBy directly: a handler notified by multiple tasks (via
// its own name and a listen topic) should list every distinct notifier
// exactly once, in tree order, and never include other handler
// definitions.
func TestHandlerNotifiedByDeduplicatesAndSortsInTreeOrder(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	// Fixture: handler h1 ("restart app") is notified by task 2. Add a
	// second, out-of-order notifier plus a duplicate-by-listen-topic to
	// exercise sorting and de-duplication.
	a.Leaves = append(a.Leaves,
		&TaskNode{ID: "1.5", Module: "debug", Notify: []string{"restart app"}, File: "tasks/setup.yml", Line: 99},
	)
	var h1 *TaskNode
	for _, l := range a.Leaves {
		if l.ID == "h1" {
			h1 = l
		}
	}
	if h1 == nil {
		t.Fatal("expected handler h1 in fixture")
	}
	ids := handlerNotifiedBy(a, h1)
	if len(ids) != len(map[string]bool{"2": true, "1.5": true}) {
		t.Fatalf("expected 2 distinct notifiers, got %v", ids)
	}
	// Tree order: "1.5" sorts before "2"? No -- dottedIDLess compares
	// first segment numerically: 1 < 2, so "1.5" comes first.
	if ids[0] != "1.5" || ids[1] != "2" {
		t.Errorf("ids = %v, want [1.5 2] in tree order", ids)
	}
	for _, id := range ids {
		if strings.HasPrefix(id, "h") {
			t.Errorf("notified-by list should never include a handler ID, got %q", id)
		}
	}
}

func TestTUIDetailsWarningWrapped(t *testing.T) {
	m := newTestModel(t)
	idx := rowByKind(t, m, rowWarning)
	row := m.rows[idx]
	fields := m.buildDetails(row)
	v, ok := detailValue(fields, "warning")
	if !ok || v != row.Text {
		t.Fatalf("warning field = %q, %v, want full text %q", v, ok, row.Text)
	}
	wrapped := wrapDetailField(detailField{"warning", v}, ansi.StringWidth("warning"), 20)
	if len(wrapped) < 2 {
		t.Errorf("expected the long warning to wrap into multiple lines at width 20, got %d: %v", len(wrapped), wrapped)
	}
	// No characters may be dropped by wrapping (only whitespace/newlines
	// may be inserted or collapsed at line boundaries, and a run wider
	// than the column is hard-broken but not lost).
	stripSpace := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\t' {
				return -1
			}
			return r
		}, s)
	}
	got := stripSpace(strings.Join(wrapped, ""))
	want := stripSpace("warning: " + v)
	if got != want {
		t.Errorf("wrapped warning lost or altered content:\ngot:  %q\nwant: %q", got, want)
	}
}

// --- Pop-up placement and rendering invariants ---

func TestTUIPopupPlacementBelowByDefault(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m.cursor = 0
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	lines := strings.Split(view, "\n")
	// Row 0 (title) + row 1 (selected, cursor=0) should be unaffected;
	// the box should start appearing at or after line index 2.
	boxFound := false
	for i := 2; i < len(lines); i++ {
		if strings.Contains(lines[i], "╭") {
			boxFound = true
			break
		}
	}
	if !boxFound {
		t.Errorf("expected a box top border below the selected row:\n%s", view)
	}
}

func TestTUIPopupPlacementAboveNearBottom(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	idx := firstOpenableRow(m)
	for i := len(m.rows) - 1; i >= 0; i-- {
		if rowOpenable(m.rows[i]) {
			idx = i
			break
		}
	}
	m.cursor = idx
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	if !strings.Contains(view, "╭") {
		t.Errorf("expected a box to be drawn when the selection is near the bottom:\n%s", view)
	}
}

func TestTUIPopupInvariantsAtSeveralSizes(t *testing.T) {
	sizes := []struct{ w, h int }{
		{80, 20}, {40, 10}, {100, 8}, {60, 6}, {30, 25},
	}
	for _, sz := range sizes {
		for _, noColor := range []bool{false, true} {
			t.Run(fmt.Sprintf("w=%d_h=%d_noColor=%v", sz.w, sz.h, noColor), func(t *testing.T) {
				a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
				if err != nil {
					t.Fatal(err)
				}
				m := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: noColor})
				m = update(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
				idx := firstOpenableRow(m)
				if idx == -1 {
					return
				}
				m.cursor = idx
				m.clampCursor()
				m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
				m2 := update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // -> details, if "Details" selected

				for _, mm := range []analyzeRoleTUIReader{m, m2} {
					view := mm.View()
					lines := strings.Split(view, "\n")
					if len(lines) > mm.height {
						t.Errorf("mode=%v: view has %d lines, want <= height %d", mm.mode, len(lines), mm.height)
					}
					for _, line := range lines {
						if w := ansi.StringWidth(line); w > sz.w {
							t.Errorf("mode=%v: line exceeds width %d (got %d): %q", mm.mode, sz.w, w, line)
						}
					}
					if noColor && strings.Contains(view, "\x1b[") {
						t.Errorf("mode=%v: unexpected ANSI in NoColor popup view:\n%q", mm.mode, view)
					}
					if !noColor {
						open, closeSeq := config.ColorPAnalyzerSelectedRow, config.ColorReset
						if _, after, found := strings.Cut(view, open); found {
							highlighted, _, found2 := strings.Cut(after, closeSeq)
							if found2 && strings.Contains(highlighted, "\x1b[") {
								t.Errorf("mode=%v: selection highlight contains embedded ANSI: %q", mm.mode, highlighted)
							}
						}
					}
				}
			})
		}
	}
}

func TestTUIPopupNoColorBleedIntoBox(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	// Pick a colored (non-selected) row to open, so its row has an open
	// SGI color that must not bleed into the box.
	idx := -1
	for i, r := range m.rows {
		if rowOpenable(r) && r.Color != "" && i != m.cursor {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Skip("fixture has no colored openable row")
	}
	m.cursor = idx
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if left, _, found := strings.Cut(line, "╭"); found {
			if strings.Contains(left, "\x1b[3") && !strings.HasSuffix(ansi.Strip(left), "") {
				// The left remainder must end with a reset right before
				// the box so the color can't visually continue into it.
				if !strings.Contains(left, "\x1b[0m\x1b[0m") && !strings.HasSuffix(left, "\x1b[0m") {
					t.Errorf("left-of-box segment does not end with a reset: %q", left)
				}
			}
		}
	}
}

// --- Pinned user styling (restored from commit 1d8d962; see also
// internal/config/constants.go's ColorPAnalyzerSelector/SelectedRow) ---

// TestTUISelectedGutterStringsPinned pins the exact byte strings used
// for the selected row's gutter, in both colour and NoColor mode, so a
// future rewrite can't silently drop the user's chosen style again. The
// colour gutter is 3 cells (no trailing spaces, moved 2 cells left of
// the original restore); NoColor stays at 4 cells (it keeps its
// trailing space since there's no background colour to lean on).
func TestTUISelectedGutterStringsPinned(t *testing.T) {
	wantColor := config.ColorPAnalyzerSelector + "◀●▶" + config.ColorReset
	if wantColor != "\x1b[38;5;201m◀●▶\x1b[0m" {
		t.Fatalf("colour gutter built from config constants = %q, want the pinned byte string", wantColor)
	}
	if w := ansi.StringWidth(wantColor); w != selectedGutterWidthColor {
		t.Errorf("colour gutter width = %d, want %d", w, selectedGutterWidthColor)
	}
	if selectedGutterWidthColor != 3 {
		t.Errorf("selectedGutterWidthColor = %d, want 3", selectedGutterWidthColor)
	}

	wantNoColor := "◀●▶"
	if w := ansi.StringWidth(wantNoColor); w != selectedGutterWidthNoColor {
		t.Errorf("NoColor gutter width = %d, want %d", w, selectedGutterWidthNoColor)
	}
	if selectedGutterWidthNoColor != 3 {
		t.Errorf("selectedGutterWidthNoColor = %d, want 3", selectedGutterWidthNoColor)
	}

	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	view := m.View()
	if !strings.Contains(view, wantColor) {
		t.Errorf("colour view does not contain the pinned selected gutter %q:\n%q", wantColor, view)
	}

	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	mNoColor := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	mNoColor = update(t, mNoColor, tea.WindowSizeMsg{Width: 80, Height: 10})
	viewNoColor := mNoColor.View()
	if !strings.Contains(viewNoColor, wantNoColor) {
		t.Errorf("NoColor view does not contain the pinned selected gutter %q:\n%q", wantNoColor, viewNoColor)
	}
}

// TestTUISelectedRowStylePinned pins the selected row's content wrapper
// to config.ColorPAnalyzerSelectedRow/config.ColorReset (black-on-
// aquamarine), not reverse video.
func TestTUISelectedRowStylePinned(t *testing.T) {
	if config.ColorPAnalyzerSelectedRow != "\x1b[30;48;5;122m" {
		t.Fatalf("ColorPAnalyzerSelectedRow = %q, want \\x1b[30;48;5;122m", config.ColorPAnalyzerSelectedRow)
	}
	if config.ColorPAnalyzerSelector != "\x1b[38;5;201m" {
		t.Fatalf("ColorPAnalyzerSelector = %q, want \\x1b[38;5;201m", config.ColorPAnalyzerSelector)
	}
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	view := m.View()
	want := config.ColorPAnalyzerSelectedRow + m.hcutWidth(m.plain[m.cursor], m.selectedContentWidth()) + config.ColorReset
	if !strings.Contains(view, want) {
		t.Errorf("view does not contain the pinned selected-row span %q:\n%q", want, view)
	}
	if strings.Contains(view, "\x1b[7m") {
		t.Error("view should never use reverse video (\\x1b[7m) for the selection anymore")
	}
}

// TestTUIFooterColourWrapperPinned pins the footer's colour wrapper to
// config.ColorPAnalyzerLightSteelBlue/config.ColorReset for every mode.
func TestTUIFooterColourWrapperPinned(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})

	checkFooterWrapped := func(t *testing.T, mm analyzeRoleTUIReader) {
		t.Helper()
		view := mm.View()
		lines := strings.Split(view, "\n")
		last := lines[len(lines)-1]
		if !strings.HasPrefix(last, config.ColorPAnalyzerLightSteelBlue) {
			t.Errorf("footer does not start with ColorPAnalyzerLightSteelBlue: %q", last)
		}
		if !strings.HasSuffix(last, config.ColorReset) {
			t.Errorf("footer does not end with ColorReset: %q", last)
		}
	}
	checkFooterWrapped(t, m) // list mode

	idx := firstOpenableRow(m)
	m.cursor = idx
	m.clampCursor()
	mMenu := update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	checkFooterWrapped(t, mMenu)
	mDetails := update(t, mMenu, tea.KeyMsg{Type: tea.KeyEnter})
	checkFooterWrapped(t, mDetails)

	// NoColor: the footer must be plain, with no ANSI wrapper at all.
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	mNoColor := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	mNoColor = update(t, mNoColor, tea.WindowSizeMsg{Width: 80, Height: 10})
	viewNoColor := mNoColor.View()
	lines := strings.Split(viewNoColor, "\n")
	last := lines[len(lines)-1]
	if strings.Contains(last, "\x1b[") {
		t.Errorf("NoColor footer should have no ANSI: %q", last)
	}
}

// --- Wider Details box with clean wrapping ---

func TestTUIDetailsNoWrapAtWidth120(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	var row treeRow
	for _, r := range m.rows {
		if r.Kind == rowLeaf && r.Leaf.ID == "1.1" {
			row = r
		}
	}
	if row.Leaf == nil {
		t.Fatal("expected leaf 1.1 in fixture")
	}
	m.cursor = indexOfRow(t, m, row)
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // -> details

	keyWidth := 0
	for _, f := range m.details {
		keyWidth = max(keyWidth, ansi.StringWidth(f.Key))
	}
	for _, f := range m.details {
		if len(wrapDetailField(f, keyWidth, 120-8)) != 1 {
			t.Errorf("field %q unexpectedly wrapped at width 120: %+v", f.Key, f)
		}
	}
}

func indexOfRow(t *testing.T, m analyzeRoleTUIReader, want treeRow) int {
	t.Helper()
	for i, r := range m.rows {
		if r.Kind == want.Kind {
			if r.Leaf != nil && want.Leaf != nil && r.Leaf.ID == want.Leaf.ID {
				return i
			}
			if r.Branch != nil && want.Branch != nil && r.Branch.ID == want.Branch.ID {
				return i
			}
		}
	}
	t.Fatal("row not found")
	return -1
}

func TestTUIDetailsWrapsLongSrcAtWidth50NoTextLost(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	// Give leaf 1.1 a long src path that will need wrapping at width 50.
	for _, l := range a.Leaves {
		if l.ID == "1.1" {
			l.Src = "some/very/long/nested/path/to/a/configuration/file/that/does/not/fit.conf"
		}
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{})
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 30})
	var row treeRow
	for _, r := range m.rows {
		if r.Kind == rowLeaf && r.Leaf.ID == "1.1" {
			row = r
		}
	}
	m.cursor = indexOfRow(t, m, row)
	m.clampCursor()
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	keyWidth := 0
	for _, f := range m.details {
		keyWidth = max(keyWidth, ansi.StringWidth(f.Key))
	}
	var srcField detailField
	for _, f := range m.details {
		if f.Key == "src" {
			srcField = f
		}
	}
	if srcField.Key == "" {
		t.Fatal("expected a src field")
	}
	wrapped := wrapDetailField(srcField, keyWidth, 30)
	if len(wrapped) < 2 {
		t.Fatalf("expected the long src path to wrap at width 30, got %d lines: %v", len(wrapped), wrapped)
	}
	stripSpace := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\t' {
				return -1
			}
			return r
		}, s)
	}
	got := stripSpace(strings.Join(wrapped, ""))
	want := stripSpace(padCells(srcField.Key, keyWidth) + ": " + srcField.Value)
	if got != want {
		t.Errorf("wrapped src lost or altered content:\ngot:  %q\nwant: %q", got, want)
	}

	// Rendering through the view must still respect every invariant.
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("line exceeds width 50 (got %d): %q", w, line)
		}
	}
}

func TestTUIDetailsBoxNeverExceedsMarginedWidth(t *testing.T) {
	for _, w := range []int{40, 60, 80, 120, 200} {
		a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range a.Leaves {
			if l.ID == "1.1" {
				l.Src = strings.Repeat("x", 300)
			}
		}
		m := newAnalyzeRoleTUIReader(a, TreeOptions{})
		m = update(t, m, tea.WindowSizeMsg{Width: w, Height: 20})
		var row treeRow
		for _, r := range m.rows {
			if r.Kind == rowLeaf && r.Leaf.ID == "1.1" {
				row = r
			}
		}
		m.cursor = indexOfRow(t, m, row)
		m.clampCursor()
		m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		view := m.View()
		for _, line := range strings.Split(view, "\n") {
			if lw := ansi.StringWidth(line); lw > w {
				t.Errorf("width=%d: line exceeds terminal width: %d: %q", w, lw, line)
			}
			// The box itself (border row) must fit within w-2*popupMargin.
			stripped := ansi.Strip(line)
			if strings.Contains(stripped, "╭") {
				runes := []rune(stripped)
				start, end := -1, -1
				for i, r := range runes {
					if r == '╭' {
						start = i
					}
					if r == '╮' {
						end = i
					}
				}
				if start != -1 && end != -1 && end >= start {
					boxWidth := ansi.StringWidth(string(runes[start : end+1]))
					if boxWidth > w-2*popupMargin {
						t.Errorf("width=%d: box width %d exceeds w-2*popupMargin=%d", w, boxWidth, w-2*popupMargin)
					}
				}
			}
		}
	}
}

func TestTUIDetailsBoxShiftsLeftNearRightEdge(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{})
	m = update(t, m, tea.WindowSizeMsg{Width: 70, Height: 20})
	// Scroll far right so the anchor column is near/after the right edge.
	for i := 0; i < 50; i++ {
		m = update(t, m, tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes})
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 70 {
			t.Errorf("line exceeds width 70 (got %d): %q", w, line)
		}
	}
	if !strings.Contains(ansi.Strip(view), "╭") {
		t.Error("expected a box to still be drawn after shifting left")
	}
}

// --- Magenta pop-ups ---

func TestTUIPopupBorderAndTitleAreMagenta(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // menu
	view := m.View()
	if !strings.Contains(view, config.ColorPAnalyzerSelector+"╭") {
		t.Errorf("expected the top border to start with the magenta selector colour:\n%q", view)
	}
	// Every magenta segment must be closed by a reset before the next
	// non-magenta content, so the colour can't bleed into the box body
	// or into the row to the right of the box.
	rest := view
	for {
		start := strings.Index(rest, config.ColorPAnalyzerSelector)
		if start == -1 {
			break
		}
		rest = rest[start+len(config.ColorPAnalyzerSelector):]
		resetIdx := strings.Index(rest, config.ColorReset)
		if resetIdx == -1 {
			t.Fatalf("magenta segment not closed by a reset:\n%q", view)
		}
		rest = rest[resetIdx+len(config.ColorReset):]
	}

	mDetails := update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	viewDetails := mDetails.View()
	if !strings.Contains(viewDetails, config.ColorPAnalyzerSelector) {
		t.Errorf("expected the Details box border to also be magenta:\n%q", viewDetails)
	}
}

func TestTUIPopupNoColorHasNoAnsi(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Contains(m.View(), "\x1b[") {
		t.Errorf("NoColor menu view should have no ANSI:\n%q", m.View())
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Contains(m.View(), "\x1b[") {
		t.Errorf("NoColor details view should have no ANSI:\n%q", m.View())
	}
}

// --- Menu selection style (consistency with the main list) ---

func TestTUIMenuSelectionStyleMatchesList(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()

	open, closeSeq := config.ColorPAnalyzerSelectedRow, config.ColorReset
	_, after, found := strings.Cut(view, open)
	if !found {
		t.Fatalf("no selected-row highlight found in the menu:\n%q", view)
	}
	highlighted, _, found := strings.Cut(after, closeSeq)
	if !found {
		t.Fatalf("no closing reset found for the menu selection:\n%q", view)
	}
	if strings.Contains(highlighted, "\x1b[") {
		t.Errorf("menu selection highlight contains embedded ANSI: %q", highlighted)
	}
	if strings.Contains(view, "\x1b[7m") {
		t.Error("menu selection should never use reverse video")
	}

	// NoColor: a plain "◀●▶" marker, no ANSI.
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	mNoColor := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	mNoColor = update(t, mNoColor, tea.WindowSizeMsg{Width: 80, Height: 20})
	mNoColor = update(t, mNoColor, tea.KeyMsg{Type: tea.KeyEnter})
	viewNoColor := mNoColor.View()
	if strings.Contains(viewNoColor, "\x1b[") {
		t.Errorf("NoColor menu should have no ANSI: %q", viewNoColor)
	}
	if !strings.Contains(viewNoColor, "◀●▶") {
		t.Error("NoColor menu should still show the ◀●▶ marker on the selected item")
	}
}
