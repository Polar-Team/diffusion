// patch_tui.go implements an interactive Bubble Tea viewer for the output
// of PrintTree, used by `diffusion patch analyze --format tree` when
// stdin/stdout are terminals. It never mutates RoleAnalysis state; it only
// renders the same lines PrintTree would print, with cursor navigation.
package patch

import (
	"bytes"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// defaultTUIHeight/defaultTUIWidth are used for layout before the first
// WindowSizeMsg arrives (e.g. while Bubble Tea negotiates the terminal
// size), and are also used as the stable layout until a real size is
// known so an early render never glitches on a transient zero size.
const (
	defaultTUIHeight = 20
	defaultTUIWidth  = 120
)

// titleStyleOn/footerStyleOn/styleOff give the title and footer a light
// visual distinction from the body when colors are enabled. They are
// plain text when opts.NoColor is set.
const (
	titleStyleOn  = "\x1b[1m" // bold
	footerStyleOn = "\x1b[2m" // dim
	styleOff      = "\x1b[0m"
)

// footerHelp is the static key-help portion of the footer line.
const footerHelp = "j/k/↑/↓ move • g/G/home/end • ctrl+d/u/pgup/pgdn • q/esc/ctrl+c quit"

// analyzeRoleTUIReader is the Bubble Tea model backing RunTreeTUI. It
// holds two parallel line slices (colored and plain) built once from a
// single PrintTree pass, plus cursor/viewport state.
type analyzeRoleTUIReader struct {
	title   string
	colored []string
	plain   []string

	cursor int
	offset int

	width  int
	height int // last known terminal height (rows)

	noColor bool
	ready   bool // true once a real tea.WindowSizeMsg has been received
}

// newAnalyzeRoleTUIReader builds the model from a single PrintTree pass:
// colored holds exactly what PrintTree would write for opts, and plain is
// its ANSI-stripped twin used for the selection highlight (so a partial
// escape sequence embedded in warning or branch text can never leak into,
// and break, the reverse-video selection). Rendering colored and plain
// from the same pass guarantees they always have identical length and
// line-for-line content, with no possibility of drift between two
// separate PrintTree calls. The first line (the header) is pulled out as
// a fixed title; the rest become the scrollable body.
func newAnalyzeRoleTUIReader(a *RoleAnalysis, opts TreeOptions) analyzeRoleTUIReader {
	var buf bytes.Buffer
	a.PrintTree(&buf, opts)
	colored := splitLines(buf.String())

	plain := make([]string, len(colored))
	for i, l := range colored {
		plain[i] = ansi.Strip(l)
	}

	title := ""
	if len(colored) > 0 {
		title = colored[0]
		colored = colored[1:]
		plain = plain[1:]
	}

	return analyzeRoleTUIReader{
		title:   title,
		colored: colored,
		plain:   plain,
		width:   defaultTUIWidth,
		height:  defaultTUIHeight,
		noColor: opts.NoColor,
	}
}

// splitLines splits s on "\n" and drops a single trailing empty element
// produced by a trailing newline (PrintTree always ends with one via
// fmt.Fprintln).
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// RunTreeTUI launches the interactive tree viewer for a, using the
// alternate screen so the terminal is fully restored on exit.
func RunTreeTUI(a *RoleAnalysis, opts TreeOptions) error {
	if a == nil {
		return fmt.Errorf("patch: cannot launch TUI for nil analysis")
	}
	m := newAnalyzeRoleTUIReader(a, opts)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// Init satisfies tea.Model.
func (m analyzeRoleTUIReader) Init() tea.Cmd {
	return nil
}

// layoutHeight returns the height used for layout: the last known
// terminal height once ready, or the default otherwise. This keeps an
// early render (before the first WindowSizeMsg) from glitching on a
// transient zero/negative size.
func (m analyzeRoleTUIReader) layoutHeight() int {
	if !m.ready {
		return defaultTUIHeight
	}
	if m.height < 0 {
		return 0
	}
	return m.height
}

// layout splits the available rows into title/body/footer, dropping the
// title when the terminal is too short for it (<3 rows) and the footer
// when it is too short for that too (<2 rows). The remaining rows (which
// may be 0) go to the scrolling body.
func (m analyzeRoleTUIReader) layout() (titleRows, bodyRows, footerRows int) {
	h := m.layoutHeight()
	h = max(h, 0)

	if h >= 3 {
		titleRows = 1
	}
	if h >= 2 {
		footerRows = 1
	}
	bodyRows = h - titleRows - footerRows
	bodyRows = max(bodyRows, 0)
	return
}

// bodyHeight returns the number of body rows actually rendered by View.
func (m analyzeRoleTUIReader) bodyHeight() int {
	_, body, _ := m.layout()
	return body
}

// scrollHeight is bodyHeight clamped to at least 1, used for cursor/
// offset arithmetic so a 0-row body (a terminal too small to show any
// content) doesn't divide by zero or leave the cursor stuck.
func (m analyzeRoleTUIReader) scrollHeight() int {
	h := m.bodyHeight()
	h = max(h, 0)
	return h
}

// clampCursor keeps the cursor within [0, len-1] and adjusts offset so the
// cursor stays visible within the current viewport.
func (m *analyzeRoleTUIReader) clampCursor() {
	if len(m.plain) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.plain)-1 {
		m.cursor = len(m.plain) - 1
	}
	h := m.scrollHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	maxOffset := len(m.plain) - h
	maxOffset = max(maxOffset, 0)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// Update satisfies tea.Model.
func (m analyzeRoleTUIReader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.clampCursor()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "j", "down":
			m.cursor++
			m.clampCursor()
		case "k", "up":
			m.cursor--
			m.clampCursor()
		case "g", "home":
			m.cursor = 0
			m.clampCursor()
		case "G", "end":
			m.cursor = len(m.plain) - 1
			m.clampCursor()
		case "ctrl+d", "pgdown":
			m.cursor += halfPageStep(m.scrollHeight())
			m.clampCursor()
		case "ctrl+u", "pgup":
			m.cursor -= halfPageStep(m.scrollHeight())
			m.clampCursor()
		}
	}
	return m, nil
}

// halfPageStep returns half of h, rounded down but never less than 1, so
// ctrl+d/ctrl+u/pgdown/pgup always move the cursor on a viewport too
// short for a true half-page.
func halfPageStep(h int) int {
	step := h / 2
	step = max(step, 0)
	return step
}

// styled wraps s in on/off unless noColor is set.
func styled(s, on string, noColor bool) string {
	if noColor || on == "" {
		return s
	}
	return on + s + styleOff
}

// footerLine composes the footer: help text on the left, "cursor/total"
// right-aligned. Only the help text is truncated (from the right, with
// an ellipsis) so the position indicator stays fully visible even on a
// narrow terminal. Widths are measured with ansi.StringWidth so the
// (ANSI-free) help/position text lines up regardless of wide runes.
func (m analyzeRoleTUIReader) footerLine(width int) string {
	pos := "0/0"
	if len(m.plain) > 0 {
		pos = fmt.Sprintf("%d/%d", m.cursor+1, len(m.plain))
	}
	posWidth := ansi.StringWidth(pos)
	availHelp := width - posWidth - 1 // reserve at least one separating space
	availHelp = max(availHelp, 0)
	help := ansi.Truncate(footerHelp, availHelp, "…")
	gap := width - ansi.StringWidth(help) - posWidth
	gap = max(gap, 0)
	return help + strings.Repeat(" ", gap) + pos
}

// View satisfies tea.Model.
func (m analyzeRoleTUIReader) View() string {
	width := m.width
	if !m.ready || width <= 0 {
		width = defaultTUIWidth
	}
	titleRows, bodyRows, footerRows := m.layout()

	var lines []string
	if titleRows > 0 {
		lines = append(lines, ansi.Truncate(styled(m.title, titleStyleOn, m.noColor), width, "…"))
	}

	end := m.offset + bodyRows
	end = min(end, len(m.plain))

	shown := 0
	for i := m.offset; i < end; i++ {
		gutter := "  "
		var line string
		switch {
		case i == m.cursor && m.noColor:
			// No reverse video in NoColor mode: the gutter alone marks
			// the cursor, and the row is otherwise plain text.
			gutter = "▸ "
			line = m.plain[i]
		case i == m.cursor:
			gutter = "▸ "
			line = "\x1b[7m" + m.plain[i] + "\x1b[0m"
		default:
			line = m.colored[i]
		}
		lines = append(lines, ansi.Truncate(gutter+line, width, "…"))
		shown++
	}
	// Pad remaining body rows so the footer stays pinned at the bottom.
	for ; shown < bodyRows; shown++ {
		lines = append(lines, "")
	}

	if footerRows > 0 {
		lines = append(lines, styled(m.footerLine(width), footerStyleOn, m.noColor))
	}

	return strings.Join(lines, "\n")
}
