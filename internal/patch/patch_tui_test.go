package patch

import (
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
	if !strings.Contains(view, "/") { // footer position indicator e.g. "N/N"
		t.Errorf("view missing footer position indicator:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got != m.height {
		t.Errorf("view has %d lines, want exactly m.height=%d", got, m.height)
	}
}

func TestTUIViewContent(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	m := newAnalyzeRoleTUIReader(a, TreeOptions{NoColor: true})
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	view := m.View()
	if !strings.Contains(view, "move") || !strings.Contains(view, "quit") {
		t.Errorf("view missing footer help text:\n%s", view)
	}
	if !strings.Contains(view, "role fixture.role") {
		t.Errorf("view missing title:\n%s", view)
	}

	// With NoColor, the only ANSI sequences allowed are the reverse-video
	// highlight on the selected line ("\x1b[7m" ... "\x1b[0m").
	stripped := view
	stripped = strings.ReplaceAll(stripped, "\x1b[7m", "")
	stripped = strings.ReplaceAll(stripped, "\x1b[0m", "")
	if strings.Contains(stripped, "\x1b[") {
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
		start := strings.Index(view, open)
		if start == -1 {
			t.Fatalf("row %d: no reverse-video start found in view:\n%s", i, view)
		}
		rest := view[start+len(open):]
		end := strings.Index(rest, closeSeq)
		if end == -1 {
			t.Fatalf("row %d: no reverse-video end found in view:\n%s", i, view)
		}
		highlighted := rest[:end]
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
	for _, h := range []int{0, 1, 2} {
		t.Run("height="+strconv.Itoa(h), func(t *testing.T) {
			m := newTestModel(t)
			m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: h})
			view := m.View()
			lineCount := 0
			if view != "" {
				lineCount = strings.Count(view, "\n") + 1
			}
			if lineCount > h {
				t.Errorf("height=%d: view has %d lines (> height):\n%q", h, lineCount, view)
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
	if maxOffset < 0 {
		maxOffset = 0
	}
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
	for _, line := range strings.Split(view, "\n") {
		visible := ansi.Strip(line)
		if len([]rune(visible)) > 20 {
			t.Errorf("line exceeds width 20 (%d runes): %q", len([]rune(visible)), visible)
		}
	}
}
