// patch_tui.go implements an interactive Bubble Tea viewer for the
// structured rows built by buildTreeRows, used by
// `diffusion patch analyze --format tree` when stdin/stdout are
// terminals. It never mutates RoleAnalysis state. Rows are shared with
// PrintTree (see patch_analyzer.go); the TUI renders them in a slimmer
// format and adds a floating Details pop-up reachable via enter/space.
package patch

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"diffusion/internal/config"

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

// listGutter is the left indent prefixed to every unselected body row;
// change it to move the whole list left/right. It is plain ASCII spaces,
// so its byte length equals its width in cells. It is never scrolled
// horizontally.
const listGutter = "   "

// gutterWidth is the width (in cells) of listGutter.
const gutterWidth = len(listGutter)

// selectedGutterWidthColor/selectedGutterWidthNoColor are the widths (in
// cells) of the selected row's gutter, which differs from listGutter:
//   - colour: "\x1b[38;5;201m◀●▶\x1b[0m" (3-cell glyph, no trailing
//     spaces), so the black-on-aquamarine block starts right after the
//     cursor glyph, in the same column as the other rows' text;
//   - NoColor: "◀●▶ " (glyph + 1 space), which keeps the space because
//     without a background colour the glyph needs it to stay readable.
//
// These are measured with ansi.StringWidth, not assumed.
const (
	selectedGutterWidthColor   = 3
	selectedGutterWidthNoColor = 4
)

// horizontalScrollStep is how many cells h/l (left/right) move the
// shared horizontal offset per key press.
const horizontalScrollStep = 8

// titleStyleOn/styleOff give the title a light visual distinction from
// the body when colors are enabled (plain text when opts.NoColor is
// set). The footer uses config.ColorPAnalyzerLightSteelBlue directly
// (see footerLine), matching the user's chosen style.
const (
	titleStyleOn = "\x1b[1m" // bold
	styleOff     = "\x1b[0m"
)

// footerHelpList/Menu/Details are the static key-help portions of the
// footer line, one per uiMode.
const (
	footerHelpList    = "j/k/↑/↓ move • h/l/←/→ scroll • 0/$ col start/end • enter/space menu • g/G/home/end • ctrl+d/u/pgup/pgdn • q/esc/ctrl+c quit"
	footerHelpMenu    = "j/k/↑/↓ move • enter/space select • esc/h/← back • q close • ctrl+c quit"
	footerHelpDetails = "j/k/↑/↓ scroll • esc back • q close • ctrl+c quit"
)

// popupMinContentWidth is the smallest content width a pop-up box will
// use, even on a narrow terminal (the box itself is still clamped to the
// terminal width).
const popupMinContentWidth = 10

// uiMode is the TUI's modal state: list browsing, the row context menu,
// or the Details pop-up.
type uiMode int

const (
	modeList uiMode = iota
	modeMenu
	modeDetails
)

// detailField is one "key: value" line of a Details pop-up, before
// wrapping (wrapping happens at render time against the live box width).
type detailField struct {
	Key   string
	Value string
}

// analyzeRoleTUIReader is the Bubble Tea model backing RunTreeTUI. rows,
// colored and plain are parallel slices (one entry per body row, header
// excluded) built once from a single buildTreeRows pass.
type analyzeRoleTUIReader struct {
	a    *RoleAnalysis
	opts TreeOptions

	title   string
	rows    []treeRow
	colored []string
	plain   []string

	cursor  int
	offset  int
	xOffset int // shared horizontal scroll offset (cells), applied to every body line

	// widestLineCells is the width (in cells) of the widest body line,
	// precomputed once so maxXOffset doesn't rescan every line on every
	// keypress/resize.
	widestLineCells int

	width  int
	height int // last known terminal height (rows)

	noColor bool
	ready   bool // true once a real tea.WindowSizeMsg has been received

	mode          uiMode
	popupRow      int // row index (into rows/colored/plain) the pop-up is anchored to
	menuItems     []string
	menuCursor    int
	details       []detailField
	detailsScroll int
}

// newAnalyzeRoleTUIReader builds the model from a single buildTreeRows
// pass: the header row becomes the fixed title, and every other row
// (except rowNotifyRef -- the user decided handlers should only appear
// as their own hX rows under "handlers:", not as extra "↳" sub-rows
// under the tasks/branches that notify them; PrintTree still renders
// those for the plain tree) is formatted once in SLIM form (colored)
// with its ANSI-stripped twin (plain) used for the selection highlight
// and width measurements, so a stray escape sequence can never leak
// into (and break) the selection or the width accounting.
func newAnalyzeRoleTUIReader(a *RoleAnalysis, opts TreeOptions) analyzeRoleTUIReader {
	all := buildTreeRows(a, opts)
	title := ""
	var body []treeRow
	if len(all) > 0 {
		title = all[0].Text
		for _, r := range all[1:] {
			if r.Kind == rowNotifyRef {
				continue
			}
			body = append(body, r)
		}
	}

	colored := make([]string, len(body))
	for i, row := range body {
		colored[i] = formatSlim(row)
	}
	plain := make([]string, len(colored))
	for i, l := range colored {
		plain[i] = ansi.Strip(l)
	}

	widest := 0
	for _, p := range plain {
		if w := ansi.StringWidth(p); w > widest {
			widest = w
		}
	}

	return analyzeRoleTUIReader{
		a:               a,
		opts:            opts,
		title:           title,
		rows:            body,
		colored:         colored,
		plain:           plain,
		widestLineCells: widest,
		width:           defaultTUIWidth,
		height:          defaultTUIHeight,
		noColor:         opts.NoColor,
	}
}

// formatSlim renders row in the TUI's slim list format: tree
// prefix/glyph, ID and name only (module/file/line/src/dest/notify/
// tags/overlays move to the Details pop-up). rowNotifyRef never reaches
// here: those rows are filtered out before the slim slices are built
// (see newAnalyzeRoleTUIReader). Colour is always embedded
// (row.Color/row.Match are the real values regardless of NoColor); the
// plain twin is derived by stripping ANSI from this output, so the two
// can never drift.
func formatSlim(row treeRow) string {
	switch row.Kind {
	case rowBranch:
		b := row.Branch
		text := b.Name
		if text == "" {
			switch {
			case b.Target != "":
				text = b.Kind + " → " + b.Target
			case b.Opaque:
				text = b.Kind + " (opaque)"
			default:
				text = b.Kind
			}
		}
		line := row.Prefix + "▶ " + b.ID + " " + text
		if row.Match {
			line += colorize(" ★", "\033[32m", false)
		}
		return colorize(line, b.Color, false)
	case rowLeaf:
		l := row.Leaf
		text := l.Name
		if text == "" {
			text = "[" + l.Module + "]"
		}
		line := row.Prefix + "● " + l.ID + " " + text
		if row.Match {
			line += colorize(" ★", "\033[32m", false)
		}
		return colorize(line, row.Color, false)
	case rowSection:
		return row.Text
	case rowWarning:
		return "  - " + colorize(row.Text, config.ColorYellow, false)
	default:
		return row.Text
	}
}

// rowOpenable reports whether enter/space should open the context menu
// for row: header and section rows ("handlers:"/"warnings:") carry no
// per-row detail and are a no-op. rowNotifyRef rows never reach the TUI
// list at all (filtered in newAnalyzeRoleTUIReader), so they are not
// listed here.
func rowOpenable(row treeRow) bool {
	switch row.Kind {
	case rowBranch, rowLeaf, rowWarning:
		return true
	default:
		return false
	}
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

// effectiveWidth returns the width used for layout: the last known
// terminal width once ready, or the default otherwise. Mirrors
// layoutHeight's handling of the pre-ready/transient-zero case.
func (m analyzeRoleTUIReader) effectiveWidth() int {
	if !m.ready || m.width <= 0 {
		return defaultTUIWidth
	}
	return m.width
}

// contentWidth is the number of cells available for scrollable line
// content, i.e. the terminal width minus the fixed gutter.
func (m analyzeRoleTUIReader) contentWidth() int {
	return max(m.effectiveWidth()-gutterWidth, 0)
}

// selectedGutterWidth is the width (in cells) of the selected row's
// wider gutter (see selectedGutterWidthColor/NoColor).
func (m analyzeRoleTUIReader) selectedGutterWidth() int {
	if m.noColor {
		return selectedGutterWidthNoColor
	}
	return selectedGutterWidthColor
}

// selectedContentWidth is the number of cells available for the
// selected row's content: the terminal width minus its wider gutter.
// The selected row deliberately starts further right than the other
// rows (the "jump" look the gutter is designed to give), so it gets a
// narrower content window than contentWidth.
func (m analyzeRoleTUIReader) selectedContentWidth() int {
	return max(m.effectiveWidth()-m.selectedGutterWidth(), 0)
}

// maxXOffset is the largest horizontal offset that still shows some new
// content: once both the widest body line (in the normal content width)
// and the currently selected line (in its narrower selected content
// width) already fit, this is 0 and h/l become no-ops. The selected
// line needs its own check because its content window is narrower than
// the other rows', so "$" must be able to scroll further to reveal its
// true end.
func (m analyzeRoleTUIReader) maxXOffset() int {
	mx := max(m.widestLineCells-m.contentWidth(), 0)
	if m.cursor >= 0 && m.cursor < len(m.plain) {
		selWidth := ansi.StringWidth(m.plain[m.cursor])
		mx = max(mx, selWidth-m.selectedContentWidth())
	}
	return max(mx, 0)
}

// clampXOffset keeps xOffset within [0, maxXOffset()].
func (m *analyzeRoleTUIReader) clampXOffset() {
	if m.xOffset < 0 {
		m.xOffset = 0
	}
	if mx := m.maxXOffset(); m.xOffset > mx {
		m.xOffset = mx
	}
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
	h = max(h, 1)
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
		m.clampXOffset()
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeDetails:
			return m.updateDetails(key)
		case modeMenu:
			return m.updateMenu(key)
		default:
			return m.updateList(key)
		}
	}
	return m, nil
}

// updateList handles keys in modeList: cursor movement, horizontal
// scroll, and opening the context menu on enter/space.
func (m analyzeRoleTUIReader) updateList(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
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
	case "h", "left":
		m.xOffset -= horizontalScrollStep
		m.clampXOffset()
	case "l", "right":
		m.xOffset += horizontalScrollStep
		m.clampXOffset()
	case "0":
		m.xOffset = 0
	case "$":
		if len(m.plain) > 0 {
			lineWidth := ansi.StringWidth(m.plain[m.cursor])
			m.xOffset = lineWidth - m.selectedContentWidth()
			m.clampXOffset()
		}
	case "enter", " ":
		if len(m.rows) > 0 && m.cursor < len(m.rows) && rowOpenable(m.rows[m.cursor]) {
			m.mode = modeMenu
			m.popupRow = m.cursor
			m.menuItems = []string{"Details"}
			m.menuCursor = 0
		}
	}
	return m, nil
}

// updateMenu handles keys in modeMenu: item navigation, activation, and
// closing back to the list.
func (m analyzeRoleTUIReader) updateMenu(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		m.menuCursor = min(m.menuCursor+1, len(m.menuItems)-1)
	case "k", "up":
		m.menuCursor = max(m.menuCursor-1, 0)
	case "enter", " ":
		if m.menuCursor >= 0 && m.menuCursor < len(m.menuItems) {
			switch m.menuItems[m.menuCursor] {
			case "Details":
				m.details = m.buildDetails(m.rows[m.popupRow])
				m.detailsScroll = 0
				m.mode = modeDetails
			}
		}
	case "esc", "backspace", "h", "left", "q":
		m.mode = modeList
	}
	return m, nil
}

// updateDetails handles keys in modeDetails: scrolling the box contents,
// going back one step to the menu (esc), or closing everything (q).
func (m analyzeRoleTUIReader) updateDetails(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		m.detailsScroll++
		if m.detailsScroll < 0 {
			m.detailsScroll = 0
		}
	case "k", "up":
		m.detailsScroll--
		if m.detailsScroll < 0 {
			m.detailsScroll = 0
		}
	case "esc":
		m.mode = modeMenu
	case "q":
		m.mode = modeList
	}
	return m, nil
}

// halfPageStep returns half of h, rounded down but never less than 1, so
// ctrl+d/ctrl+u/pgdown/pgup always move the cursor on a viewport too
// short for a true half-page.
func halfPageStep(h int) int {
	step := h / 2
	step = max(step, 1)
	return step
}

// styled wraps s in on/off unless noColor is set.
func styled(s, on string, noColor bool) string {
	if noColor || on == "" {
		return s
	}
	return on + s + styleOff
}

// hcut applies the model's shared horizontal scroll window to s, cutting
// the cell range [xOffset, xOffset+cw), at the given content width cw
// (unselected rows pass contentWidth(); the selected row passes its own
// narrower selectedContentWidth(), see the type doc). It then REPLACES
// (not adds) the first visible cell with "…" when something was
// scrolled off on the left, and the last visible cell with "…" when
// content continues past the right edge -- so the window's total width
// never grows past cw and scrolling all the way to the end of a line
// (via "$") still reveals its true last character instead of losing it
// to a redundant tail marker. Leading/trailing SGI escape codes in s
// survive every cut: ansi.Cut copies escape sequences unconditionally,
// independent of the visible-column bookkeeping, so a line wrapped as
// "\x1b[36m...\x1b[0m" keeps both ends after scrolling.
func (m analyzeRoleTUIReader) hcutWidth(s string, cw int) string {
	if cw <= 0 {
		return ""
	}
	total := ansi.StringWidth(s)
	if total == 0 || m.xOffset >= total {
		return "" // empty, or fully scrolled past
	}
	left := m.xOffset
	right := left + cw
	cutRight := right < total
	cutLeft := left > 0

	windowed := ansi.Cut(s, left, right)

	if cutLeft {
		if w := ansi.StringWidth(windowed); w > 0 {
			windowed = "…" + ansi.Cut(windowed, 1, w)
		}
	}
	if cutRight {
		if w := ansi.StringWidth(windowed); w > 0 {
			windowed = ansi.Cut(windowed, 0, w-1) + "…"
		}
	}
	return windowed
}

// hcut is hcutWidth at the standard (unselected-row) content width.
func (m analyzeRoleTUIReader) hcut(s string) string {
	return m.hcutWidth(s, m.contentWidth())
}

// footerHelpFor returns the static key-help text for the given mode.
func footerHelpFor(mode uiMode) string {
	switch mode {
	case modeMenu:
		return footerHelpMenu
	case modeDetails:
		return footerHelpDetails
	default:
		return footerHelpList
	}
}

// footerLine composes the footer: mode-specific help text on the left,
// then (when scrolled horizontally) "col N", then "cursor/total"
// right-aligned. Only the help text is truncated (from the right, with
// an ellipsis) so the position indicator stays fully visible even on a
// narrow terminal. Widths are measured with ansi.StringWidth so the
// (ANSI-free) help/position text lines up regardless of wide runes. In
// colour mode the whole line is wrapped in
// config.ColorPAnalyzerLightSteelBlue/config.ColorReset; NoColor leaves
// it plain.
func (m analyzeRoleTUIReader) footerLine(width int) string {
	pos := "0/0"
	if len(m.plain) > 0 {
		pos = fmt.Sprintf("%d/%d", m.cursor+1, len(m.plain))
	}
	if m.xOffset > 0 {
		pos = fmt.Sprintf("col %d  %s", m.xOffset, pos)
	}
	posWidth := ansi.StringWidth(pos)
	availHelp := width - posWidth - 1 // reserve at least one separating space
	availHelp = max(availHelp, 0)
	help := ansi.Truncate(footerHelpFor(m.mode), availHelp, "…")
	gap := width - ansi.StringWidth(help) - posWidth
	gap = max(gap, 1)
	line := help + strings.Repeat(" ", gap) + pos
	if m.noColor {
		return line
	}
	return config.ColorPAnalyzerLightSteelBlue + line + config.ColorReset
}

// padCells pads or truncates s to exactly width cells (ANSI-aware),
// right-padding with spaces when shorter.
func padCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w >= width {
		return ansi.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-w)
}

// popupMargin is the minimum gap (in cells) kept between a pop-up box
// and both screen edges: it caps the box width and clamps where the box
// can be anchored horizontally.
const popupMargin = 2

// popupBorderLine draws one rounded-corner border row, width cells wide,
// with an optional centered title (used for the top border, e.g.
// "╭─ 1.2 Details ─╮"). In colour mode the whole line (border glyphs and
// title) is wrapped in config.ColorPAnalyzerSelector/config.ColorReset,
// matching the user's magenta selector colour; NoColor leaves it plain.
func popupBorderLine(title string, width int, left, right rune, colorOn bool) string {
	inner := max(width-2, 0)
	t := ""
	if title != "" {
		t = " " + title + " "
	}
	tw := ansi.StringWidth(t)
	if tw > inner {
		t = ansi.Truncate(t, inner, "")
		tw = ansi.StringWidth(t)
	}
	leftDash := max((inner-tw)/2, 0)
	rightDash := max(inner-tw-leftDash, 0)
	line := string(left) + strings.Repeat("─", leftDash) + t + strings.Repeat("─", rightDash) + string(right)
	if !colorOn {
		return line
	}
	return config.ColorPAnalyzerSelector + line + config.ColorReset
}

// popupPipe returns the box's vertical border character, individually
// wrapped in config.ColorPAnalyzerSelector/config.ColorReset (so the
// colour never bleeds into the padded content between the two pipes) in
// colour mode, or plain in NoColor.
func popupPipe(colorOn bool) string {
	if !colorOn {
		return "│"
	}
	return config.ColorPAnalyzerSelector + "│" + config.ColorReset
}

// View satisfies tea.Model.
func (m analyzeRoleTUIReader) View() string {
	width := m.effectiveWidth()
	titleRows, bodyRows, footerRows := m.layout()

	var lines []string
	if titleRows > 0 {
		// The title is never horizontally scrolled.
		lines = append(lines, ansi.Truncate(styled(m.title, titleStyleOn, m.noColor), width, "…"))
	}

	end := m.offset + bodyRows
	end = min(end, len(m.plain))

	shown := 0
	for i := m.offset; i < end; i++ {
		gutter := listGutter
		var line string
		switch {
		case i == m.cursor && m.noColor:
			// Plain "◀●▶" marker, no ANSI at all, matching the colour
			// variant's glyph but with no highlight codes.
			gutter = "◀●▶"
			line = m.hcutWidth(m.plain[i], m.selectedContentWidth()) + "◀●▶"
		case i == m.cursor:
			gutter = config.ColorPAnalyzerSelector + "◀●▶" + config.ColorReset
			// Cut the plain line first, then wrap it in the selected-row
			// style, so the highlight is always one unbroken
			// ColorPAnalyzerSelectedRow...ColorReset span, however the
			// horizontal window lands.
			line = config.ColorPAnalyzerSelectedRow + m.hcutWidth(m.plain[i], m.selectedContentWidth()) + config.ColorReset
		default:
			if m.noColor {
				line = m.hcut(m.plain[i])
			} else {
				line = m.hcut(m.colored[i])
			}
		}
		lines = append(lines, gutter+line)
		shown++
	}
	// Pad remaining body rows so the footer stays pinned at the bottom.
	for ; shown < bodyRows; shown++ {
		lines = append(lines, "")
	}

	if m.mode != modeList {
		m.composePopup(lines, titleRows, bodyRows, width)
	}

	if footerRows > 0 {
		lines = append(lines, m.footerLine(width))
	}

	return strings.Join(lines, "\n")
}

// composePopup draws the menu or details pop-up over lines (the already-
// rendered list rows), in place. It never touches the title or footer
// rows (callers pass their indices as titleRows/bodyRows), and never
// overlaps the selected (anchor) row itself: it is placed on the row
// below the selection, or above it if there isn't enough room below, or
// in whichever space is larger (with scrollable content) if it fits
// neither way.
func (m analyzeRoleTUIReader) composePopup(lines []string, titleRows, bodyRows, width int) {
	if m.popupRow < 0 || m.popupRow >= len(m.rows) {
		return
	}
	cursorViewRow := m.popupRow - m.offset
	if cursorViewRow < 0 || cursorViewRow >= bodyRows {
		return // the anchor row itself has scrolled out of view
	}

	colorOn := !m.noColor
	title := m.popupTitle()
	var contentLines []string
	var boxWidth int

	if m.mode == modeMenu {
		desiredContentWidth := 12
		for _, it := range m.menuItems {
			desiredContentWidth = max(desiredContentWidth, ansi.StringWidth(it)+2)
		}
		boxWidth = m.popupBoxWidth(desiredContentWidth)
		innerWidth := max(boxWidth-4, 1)
		for i, item := range m.menuItems {
			gutter, text := "  ", item
			switch {
			case i == m.menuCursor && m.noColor:
				// Plain "◀●▶" marker, no ANSI at all, matching the list's
				// NoColor selection style.
				gutter = "◀●▶ "
			case i == m.menuCursor:
				gutter = config.ColorPAnalyzerSelector + "◀●▶" + config.ColorReset
				text = config.ColorPAnalyzerSelectedRow + text + config.ColorReset
			}
			contentLines = append(contentLines, padCells(gutter+text, innerWidth))
		}
	} else {
		// Details: size the box from the widest UNWRAPPED "key: value"
		// line (keys padded to a common column), capped to the terminal
		// width minus popupMargin on each side; wrapping only kicks in
		// for a value that still doesn't fit (see wrapDetailField).
		keyWidth := 0
		for _, f := range m.details {
			keyWidth = max(keyWidth, ansi.StringWidth(f.Key))
		}
		longest := 0
		for _, f := range m.details {
			longest = max(longest, keyWidth+2+ansi.StringWidth(f.Value))
		}
		boxWidth = m.detailsBoxWidth(longest)
		innerWidth := max(boxWidth-4, 1)
		for _, f := range m.details {
			contentLines = append(contentLines, wrapDetailField(f, keyWidth, innerWidth)...)
		}
	}
	innerWidth := max(boxWidth-4, 1)

	wantTotal := len(contentLines) + 2
	belowSpace := bodyRows - (cursorViewRow + 1)
	aboveSpace := cursorViewRow

	var top, height int
	scrollable := false
	switch {
	case belowSpace >= wantTotal:
		top, height = cursorViewRow+1, wantTotal
	case aboveSpace >= wantTotal:
		top, height = cursorViewRow-wantTotal, wantTotal
	case belowSpace >= aboveSpace:
		top = cursorViewRow + 1
		height = max(belowSpace, 3)
		scrollable = true
	default:
		height = max(aboveSpace, 3)
		top = cursorViewRow - height
		scrollable = true
	}
	height = min(height, bodyRows)
	if top < 0 {
		top = 0
	}
	if top+height > bodyRows {
		top = max(bodyRows-height, 0)
	}
	if height < 3 {
		return // no usable room to draw a box at all
	}

	visibleContentRows := height - 2
	if scrollable || len(contentLines) > visibleContentRows {
		maxScroll := max(len(contentLines)-visibleContentRows, 0)
		scroll := m.detailsScroll
		scroll = min(scroll, maxScroll)
		scroll = max(scroll, 0)
		endIdx := min(scroll+visibleContentRows, len(contentLines))
		contentLines = contentLines[scroll:endIdx]
	}

	pipe := popupPipe(colorOn)
	boxLines := make([]string, 0, height)
	boxLines = append(boxLines, popupBorderLine(title, boxWidth, '╭', '╮', colorOn))
	for _, cl := range contentLines {
		boxLines = append(boxLines, pipe+" "+padCells(cl, innerWidth)+" "+pipe)
	}
	for len(boxLines) < height-1 {
		boxLines = append(boxLines, pipe+" "+padCells("", innerWidth)+" "+pipe)
	}
	boxLines = append(boxLines, popupBorderLine("", boxWidth, '╰', '╯', colorOn))

	// Anchor the box's left edge at the selected row's text start
	// (selected gutter + prefix − xOffset), not sized from it: the box
	// width above is independent of this column. If it wouldn't fit to
	// the right, shift it left instead of narrowing it, down to
	// popupMargin cells from the left edge.
	desiredAnchor := m.selectedGutterWidth() + ansi.StringWidth(m.rows[m.popupRow].Prefix) - m.xOffset
	maxAnchor := max(width-popupMargin-boxWidth, popupMargin)
	anchorCol := max(desiredAnchor, popupMargin)
	anchorCol = min(anchorCol, maxAnchor)

	base := titleRows + top
	for j, boxLine := range boxLines {
		idx := base + j
		if idx < 0 || idx >= len(lines) {
			continue
		}
		lines[idx] = m.composeRow(lines[idx], anchorCol, boxWidth, boxLine)
	}
}

// composeRow splices boxLine into original at column anchorCol (which is
// exactly boxWidth cells wide), preserving whatever is left of the
// original row before anchorCol and whatever remains after
// anchorCol+boxWidth. ansi.Truncate/TruncateLeft copy any SGI escape
// sequences present in original unconditionally (independent of the
// visible-column cut), so both the left and right remainders keep the
// row's original colour; the explicit reset below is an extra safety net
// so a colour left open by "left" can never bleed into the box itself.
func (m analyzeRoleTUIReader) composeRow(original string, anchorCol, boxWidth int, boxLine string) string {
	left := ansi.Truncate(original, anchorCol, "")
	if w := ansi.StringWidth(left); w < anchorCol {
		left += strings.Repeat(" ", anchorCol-w)
	}
	if !m.noColor {
		left += "\x1b[0m"
	}
	right := ansi.TruncateLeft(original, anchorCol+boxWidth, "")
	return left + boxLine + right
}

// popupBoxWidth returns the total menu-box width (including borders and
// padding) for a desired content width, clamped to popupMargin from the
// terminal edge on each side and to a minimum workable size.
func (m analyzeRoleTUIReader) popupBoxWidth(desiredContentWidth int) int {
	maxBox := max(m.effectiveWidth()-2*popupMargin, 4)
	box := min(desiredContentWidth+4, maxBox)
	minBox := min(popupMinContentWidth+4, maxBox)
	return max(box, minBox)
}

// detailsBoxWidth returns the Details box width (including borders and
// padding) for the widest unwrapped "key: value" content line
// (longestLineWidth): at least min(60, terminal width − 2·popupMargin),
// and never more than the terminal width minus popupMargin on each
// side, so it's never cramped on a normal terminal but also never
// crowds the screen edges.
func (m analyzeRoleTUIReader) detailsBoxWidth(longestLineWidth int) int {
	maxBox := max(m.effectiveWidth()-2*popupMargin, 4)
	minBox := min(60, maxBox)
	box := max(longestLineWidth+4, minBox)
	return min(box, maxBox)
}

// popupTitle returns the pop-up's top-border title for the anchor row.
func (m analyzeRoleTUIReader) popupTitle() string {
	if m.popupRow < 0 || m.popupRow >= len(m.rows) {
		return "Details"
	}
	row := m.rows[m.popupRow]
	if m.mode == modeMenu {
		switch row.Kind {
		case rowBranch:
			return row.Branch.ID
		case rowLeaf:
			return row.Leaf.ID
		default:
			return "Menu"
		}
	}
	switch row.Kind {
	case rowBranch:
		return row.Branch.ID + " Details"
	case rowLeaf:
		return row.Leaf.ID + " Details"
	case rowWarning:
		return "Warning"
	default:
		return "Details"
	}
}

// buildDetails returns the Details pop-up's raw (unwrapped) key/value
// fields for row. rowNotifyRef never reaches here (filtered out of the
// TUI's rows).
func (m analyzeRoleTUIReader) buildDetails(row treeRow) []detailField {
	switch row.Kind {
	case rowLeaf:
		return m.leafDetails(row.Leaf)
	case rowBranch:
		return branchDetails(row.Branch)
	case rowWarning:
		return []detailField{{"warning", row.Text}}
	default:
		return nil
	}
}

// leafDetails builds the Details fields for a task or handler leaf:
// identity, file position, (handlers only) who notifies it, inherited/
// own tags with the branch chain, file evidence, notify targets
// (resolved to handler IDs), listen topics (handlers only) and any
// overlay annotation. Empty fields are skipped.
func (m analyzeRoleTUIReader) leafDetails(l *TaskNode) []detailField {
	var out []detailField
	add := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out = append(out, detailField{k, v})
		}
	}
	add("id", l.ID)
	add("name", l.Name)
	add("module", l.Module)
	add("file", l.File)
	add("line", strconv.Itoa(l.Line))
	if handler, _, err := splitTaskID(l.ID); err == nil && handler {
		if ids := handlerNotifiedBy(m.a, l); len(ids) > 0 {
			add("notified by", strings.Join(ids, ","))
		}
	}
	if ancestor := m.a.nearestTaggedBranch(l.ID); ancestor != nil {
		target := ancestor.Kind
		if ancestor.Target != "" {
			target += " → " + ancestor.Target
		}
		add("inherits", fmt.Sprintf("[%s] from %s (%s)", strings.Join(ancestor.Tags, ","), ancestor.ID, target))
	}
	if chain := m.a.branchChain(l.ID); len(chain) > 0 {
		parts := make([]string, len(chain))
		for i, b := range chain {
			s := b.ID + " " + b.Kind
			if b.Target != "" {
				s += " → " + b.Target
			}
			parts[i] = s
		}
		add("branch", strings.Join(parts, " › "))
	}
	if len(l.Tags) > 0 {
		add("tags", strings.Join(l.Tags, ","))
	}
	add("src", l.Src)
	add("dest", l.Dest)
	if len(l.Notify) > 0 {
		var parts []string
		for _, n := range l.Notify {
			defs := m.a.handlerDefs(n)
			if len(defs) == 0 {
				parts = append(parts, n+" (undefined)")
				continue
			}
			ids := make([]string, len(defs))
			for i, d := range defs {
				ids[i] = d.ID
			}
			parts = append(parts, n+" → "+strings.Join(ids, ","))
		}
		add("notify", strings.Join(parts, "; "))
	}
	if len(l.Listen) > 0 {
		add("listen", strings.Join(l.Listen, ","))
	}
	for id, ref := range m.opts.Overlays {
		if canon, err := canonicalAnyID(id); err == nil && canon == l.ID {
			add("overlay", "<= "+ref)
			break
		}
	}
	return out
}

// handlerNotifiedBy returns the IDs of tasks and branches that notify
// handler leaf h, either by its own definition name or by any of its
// listen topics. It scans a.Leaves and a.Branches directly (rather than
// a.Handlers, which only indexes leaf notifiers, not branch ones),
// excludes other handler-definition entries, deduplicates, and sorts in
// tree order.
func handlerNotifiedBy(a *RoleAnalysis, h *TaskNode) []string {
	if a == nil || h == nil {
		return nil
	}
	targets := map[string]bool{}
	if strings.TrimSpace(h.Name) != "" {
		targets[h.Name] = true
	}
	for _, t := range h.Listen {
		if strings.TrimSpace(t) != "" {
			targets[t] = true
		}
	}
	if len(targets) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		if handler, _, err := splitTaskID(id); err == nil && handler {
			return // exclude other handler-definition entries
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, l := range a.Leaves {
		for _, n := range l.Notify {
			if targets[n] {
				add(l.ID)
			}
		}
	}
	for _, b := range a.Branches {
		for _, n := range b.Notify {
			if targets[n] {
				add(b.ID)
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return dottedIDLess(ids[i], ids[j]) })
	return ids
}

// dottedIDLess orders dotted task/branch IDs numerically segment by
// segment (so "2" sorts before "10"), falling back to a plain string
// compare for anything that doesn't parse.
func dottedIDLess(a, b string) bool {
	_, pa, errA := splitTaskID(a)
	_, pb, errB := splitTaskID(b)
	if errA != nil || errB != nil {
		return a < b
	}
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// branchDetails builds the Details fields for a branch row.
func branchDetails(b *BranchNode) []detailField {
	var out []detailField
	add := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out = append(out, detailField{k, v})
		}
	}
	add("id", b.ID)
	add("name", b.Name)
	add("kind", b.Kind)
	add("file", b.File)
	add("line", strconv.Itoa(b.Line))
	add("target", b.Target)
	if b.Opaque {
		v := "yes"
		if b.Warning != "" {
			v += ": " + b.Warning
		}
		add("opaque", v)
	}
	if len(b.Tags) > 0 {
		add("tags", strings.Join(b.Tags, ","))
	}
	if len(b.Notify) > 0 {
		add("notify", strings.Join(b.Notify, ","))
	}
	return out
}

// detailsBreakpoints additionally break a wrapped value after /, -, _
// and . so long paths and URLs (e.g. "tasks/some/very/long/path.yml")
// wrap at sensible points instead of only at spaces; '-' is always a
// breakpoint for ansi.Wrap regardless of this set. ansi.Wrap still
// hard-breaks a single token that doesn't fit even at these points, and
// never truncates or drops characters.
const detailsBreakpoints = "/_."

// wrapDetailField wraps one "key: value" field to width cells
// (ANSI-aware, cell-counted). keyWidth is the common key column width
// (the longest key among all fields in the same box), so every field's
// value starts in the same column and keys stay aligned; continuation
// lines are indented to match. Wrapping only ever breaks lines (at word
// boundaries, or after a path/URL separator, or as a last resort mid-
// token) -- it never truncates or drops a character.
func wrapDetailField(f detailField, keyWidth, width int) []string {
	prefix := padCells(f.Key, keyWidth) + ": "
	indentW := ansi.StringWidth(prefix)
	avail := max(width-indentW, 1)
	wrapped := ansi.Wrap(f.Value, avail, detailsBreakpoints)
	parts := strings.Split(wrapped, "\n")
	indent := strings.Repeat(" ", indentW)
	out := make([]string, len(parts))
	for i, p := range parts {
		if i == 0 {
			out[i] = prefix + p
		} else {
			out[i] = indent + p
		}
	}
	return out
}
