// Package frame is enved's screen: a sidebar of tabs and a panel with
// the selected tab's content; a review of every change before saving; the save itself, off
// the UI loop because UAC may be asking; and a quit dialog that only asks about unsaved
// changes. A tab supplies its content and its changes; the frame does the rest.
package frame

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// Event is what a key in a tab asks of the frame.
type Event int

const (
	None   Event = iota
	Back         // back to the sidebar (out of a list editor: back to what holds it)
	Save         // review and save
	Reload       // read the tab again
)

// Tab is one entry of the sidebar and what the panel shows for it.
type Tab interface {
	Name() string  // in the sidebar: "User"
	Label() string // in the review and the outcome: "User PATH", "User variables"
	Count() string // in the sidebar, after the name
	Err() error    // it could not be read
	Load()         // read it again, dropping every change
	Update(tea.Msg) (tea.Cmd, Event)

	Resize(w, h int) // the body's size
	Focus(bool)      // whether the tab has the keys
	Heading() string // the panel's first line
	Body() string    // the rest of the panel
	Overlay() string // a box drawn over the screen, or ""
	Status() string  // the tab's one-off note, or ""
}

// Saver is a tab whose changes can be saved; a Tab that is not one is read-only.
type Saver interface {
	Tab
	NeedsAdmin() bool         // saving it asks for admin (UAC)
	Pending() int             // the number of changes, for the quit question
	Changes() []winenv.Change // what saving writes
	Review() []string         // the changes, one line each, for the review
	Warnings() []string       // changes to confirm a second time before saving
}

// dirty is t's changes, if it can have any: saving would write something.
func dirty(t Tab) (Saver, bool) {
	s, ok := t.(Saver)
	return s, ok && len(s.Changes()) > 0
}

// Options are what a program says about itself.
type Options struct {
	SidebarWidth int
	Apply        func([]winenv.Change) error // writes a tab's changes
	AfterSave    string                      // a note under the outcome of a save
}

// Model is the whole screen.
type Model struct {
	opts    Options
	tabs    []Tab
	saveErr []error // per tab: the last save failed, the changes are still there
	on      int     // the sidebar's selection
	inBody  bool    // focus: the tab's body (true) or the sidebar
	w, h    int     // terminal size
	status  string  // a one-off note next to the heading, cleared by the next key

	// At most one of these is open over or instead of the panel.
	confirmQuit bool     // unsaved changes: quit anyway?
	reviewing   bool     // the changes, before saving them
	confirmSave bool     // the review holds warnings: enter was pressed once
	saving      bool     // the save is running (UAC may be asking)
	outcome     []string // what the save did, shown on the review screen afterwards

	review viewport.Model // the review's lines, scrolled
	help   help.Model
}

// New is the screen over tabs, with the sidebar on tab on; open puts the keys in its body.
func New(opts Options, tabs []Tab, on int, open bool) Model {
	m := Model{opts: opts, tabs: tabs, saveErr: make([]error, len(tabs)), on: on, review: newReview(), help: help.New()}
	if open {
		m.inBody = true
		m.tab().Focus(true)
	}
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) tab() Tab { return m.tabs[m.on] }

// Tab is the tab on the sidebar's selection.
func (m Model) Tab() Tab { return m.tab() }

// InBody says whether the keys go to the selected tab's body.
func (m Model) InBody() bool { return m.inBody }

// Reviewing says whether the review (or the outcome of a save) is on screen.
func (m Model) Reviewing() bool { return m.reviewing }

// Saving says whether a save is running.
func (m Model) Saving() bool { return m.saving }

// Outcome is what the last save did, while it is on screen.
func (m Model) Outcome() []string { return m.outcome }

// ConfirmingQuit says whether the quit question is open.
func (m Model) ConfirmingQuit() bool { return m.confirmQuit }

// SaveErr is why the last save of tab i failed, or nil.
func (m Model) SaveErr(i int) error { return m.saveErr[i] }

// Status is the note next to the heading, the frame's or the tab's.
func (m Model) Status() string {
	if m.status != "" {
		return m.status
	}
	return m.tab().Status()
}

func (m *Model) resize() {
	frameW, frameH := theme.Panel.GetFrameSize()
	w, h := m.w-m.opts.SidebarWidth-frameW, m.h-frameH-1 // the heading takes a line
	for _, t := range m.tabs {
		t.Resize(max(w, 0), max(h, 0))
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resize()
		m.syncReview()
		return m, nil
	case savedMsg:
		return m.saved(msg)
	case tea.KeyPressMsg:
		m.status = ""
		if key.Matches(msg, keyForceQuit) {
			return m, tea.Quit
		}
		switch {
		case m.confirmQuit:
			return m.updateQuit(msg)
		case m.reviewing:
			return m.updateReview(msg)
		case m.inBody:
			cmd, ev := m.tab().Update(msg)
			return m.handle(ev, cmd)
		default:
			return m.updateSide(msg)
		}
	}
	cmd, _ := m.tab().Update(msg) // the cursor blink
	return m, cmd
}

// handle does what a tab's key asked.
func (m Model) handle(ev Event, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	switch ev {
	case Back:
		m.inBody = false
		m.tab().Focus(false)
	case Save:
		return m.startReview()
	case Reload:
		return m.reload()
	}
	return m, cmd
}

func (m Model) anyDirty() bool {
	return slices.ContainsFunc(m.tabs, func(t Tab) bool { _, d := dirty(t); return d })
}

// The sidebar picks a tab; enter hands the keys to its body.
func (m Model) updateSide(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyQuit):
		if m.anyDirty() {
			m.confirmQuit = true
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, keySave):
		return m.startReview()
	case key.Matches(msg, keyReload):
		return m.reload()
	case key.Matches(msg, keyUp):
		m.on = max(m.on-1, 0)
	case key.Matches(msg, keyDown):
		m.on = min(m.on+1, len(m.tabs)-1)
	case key.Matches(msg, keyOpen):
		m.inBody = true
		m.tab().Focus(true)
	}
	return m, nil
}

// reload reads the selected tab again, unless it has changes that would be lost.
func (m Model) reload() (tea.Model, tea.Cmd) {
	if _, d := dirty(m.tab()); d {
		m.status = "unsaved changes: save them, or undo them with u, first"
		return m, nil
	}
	m.tab().Load()
	m.saveErr[m.on] = nil
	m.status = "reloaded"
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.w == 0 { // the first frame comes before the terminal's size is known
		return v
	}
	v.Content = m.viewMain()
	if m.reviewing {
		v.Content = m.viewReview()
	}
	switch {
	case m.confirmQuit:
		v.Content = m.overlay(v.Content, m.quitDialog())
	case !m.reviewing && m.tab().Overlay() != "":
		v.Content = m.overlay(v.Content, m.tab().Overlay())
	}
	return v
}

// overlay draws a box over the middle of the screen.
func (m Model) overlay(under, box string) string {
	x := max((m.w-lipgloss.Width(box))/2, 0)
	y := max((m.h-lipgloss.Height(box))/2, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(under), lipgloss.NewLayer(box).X(x).Y(y).Z(1)).Render()
}

// ---- the sidebar and the panel ----------------------------------------------------------

func (m Model) viewMain() string {
	sideW := m.opts.SidebarWidth
	var side []string
	for i, t := range m.tabs {
		// The name and an admin/read-only badge on the left; the count and a flag (! or *) in fixed
		// columns on the right, so the counts line up.
		style, mark := theme.Dim, "  "
		if i == m.on {
			style, mark = theme.Accent, "▌ "
		}
		left := style.Render(mark + t.Name())
		s, isDirty := dirty(t)
		switch {
		case s == nil:
			left += " " + theme.Badge.Render("read-only")
		case s.NeedsAdmin():
			left += " " + theme.Badge.Render("admin")
		}
		count, flag := t.Count(), " "
		switch {
		case t.Err() != nil:
			count, flag = "", theme.Err.Render("!")
		case m.saveErr[i] != nil:
			flag = theme.Err.Render("!")
		case isDirty:
			flag = style.Render("*")
		}
		// The panel's border and padding take 4 cells; a space, the count 3 and the flag 1.
		inner := max(sideW-4, 0)
		row := lipgloss.PlaceHorizontal(max(inner-5, 0), lipgloss.Left, left) + style.Render(fmt.Sprintf(" %3s", count)) + flag
		side = append(side, ansi.Truncate(row, inner, ""))
	}

	focused := theme.Panel.BorderForeground(theme.ColorAccent)
	sideStyle, bodyStyle := theme.Panel, focused
	if !m.inBody {
		sideStyle, bodyStyle = focused, theme.Panel
	}
	// While it has focus, the sidebar keeps its keys at the bottom. (The filler string adds
	// one line more than its newlines.)
	if !m.inBody {
		foot := m.help.FullHelpView([][]key.Binding{{keyUp, keyDown, keyOpen, keyReload, keySave, keyQuit}})
		inner := m.h - sideStyle.GetVerticalFrameSize()
		side = append(side, strings.Repeat("\n", max(inner-len(side)-lipgloss.Height(foot)-1, 0)))
		side = append(side, foot)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		sideStyle.Width(sideW).Height(m.h).Render(strings.Join(side, "\n")),
		bodyStyle.Width(m.w-sideW).Height(m.h).Render(m.viewPanel()))
}

// viewPanel is the right panel: the tab's heading with what the frame knows about the tab,
// then its body.
func (m Model) viewPanel() string {
	t := m.tab()
	width := max(m.w-m.opts.SidebarWidth-theme.Panel.GetHorizontalFrameSize(), 0)
	heading := t.Heading()
	s, _ := t.(Saver)
	switch {
	case t.Err() != nil:
		heading += theme.Err.Render(" · could not be read: " + t.Err().Error())
	case m.saveErr[m.on] != nil:
		heading += theme.Err.Render(" · not saved: " + m.saveErr[m.on].Error())
	case s == nil:
		heading += theme.Dim.Render(" · read-only")
	case s.NeedsAdmin():
		heading += theme.Dim.Render(" · saving asks for admin (UAC)")
	}
	if s := m.Status(); s != "" {
		heading += "  " + theme.Err.Render(s)
	}
	return ansi.Truncate(heading, width, "…") + "\n" + t.Body()
}

// ---- quitting -----------------------------------------------------------------------------

func (m Model) quitDialog() string {
	n := 0
	for _, t := range m.tabs {
		if s, ok := t.(Saver); ok {
			n += s.Pending()
		}
	}
	return theme.Modal.Render(lipgloss.JoinVertical(lipgloss.Center,
		theme.Err.Bold(true).Render("Quit without saving?"), "", theme.Plural(n, "change", "changes")+" will be lost.", "",
		m.help.ShortHelpView([]key.Binding{keyQuitYes, keyQuitNo})))
}

// The quit dialog only answers the question; every other key is ignored while it is open.
func (m Model) updateQuit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyQuitYes):
		return m, tea.Quit
	case key.Matches(msg, keyQuitNo):
		m.confirmQuit = false
	}
	return m, nil
}

// ---- review and save ---------------------------------------------------------------------

func (m Model) startReview() (tea.Model, tea.Cmd) {
	if !m.anyDirty() {
		m.status = "nothing to save"
		return m, nil
	}
	m.reviewing, m.confirmSave, m.outcome = true, false, nil
	m.syncReview()
	m.review.GotoTop()
	return m, nil
}

func (m Model) warnings() []string {
	var all []string
	for _, t := range m.tabs {
		if s, d := dirty(t); d {
			all = append(all, s.Warnings()...)
		}
	}
	return all
}

// reviewLines lists every change, tab by tab, each tab's warnings first.
func (m Model) reviewLines(width int) []string {
	var lines []string
	for _, t := range m.tabs {
		s, d := dirty(t)
		if !d {
			continue
		}
		head := theme.Accent.Render(t.Label())
		if s.NeedsAdmin() {
			head += theme.Dim.Render("  saving it asks for admin: UAC will prompt")
		}
		lines = append(lines, head)
		for _, w := range s.Warnings() {
			lines = append(lines, theme.Err.Render("  ! "+w))
		}
		lines = append(lines, s.Review()...)
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return lines
}

// reviewSize is the review's scrolling area: the panel less the title, a blank, a blank and
// the help.
func (m Model) reviewSize() (w, h int) {
	return max(m.w-theme.Panel.GetHorizontalFrameSize(), 0), max(m.h-theme.Panel.GetVerticalFrameSize()-4, 1)
}

// syncReview fills the scrolling area with what is to be shown: the changes, or the outcome
// of the save.
func (m *Model) syncReview() {
	w, h := m.reviewSize()
	m.review.SetWidth(w)
	m.review.SetHeight(h)
	lines := m.outcome
	if lines == nil {
		lines = m.reviewLines(w)
	}
	m.review.SetContent(strings.Join(lines, "\n"))
}

func (m Model) viewReview() string {
	title := theme.Accent.Render("Review") + theme.Dim.Render(" · what saving will write")
	keys := []key.Binding{keyReviewSave, keyReviewBack}
	note := ""
	if len(m.warnings()) > 0 {
		note = theme.Dim.Render("it asks again: the changes marked ! need a second look · ")
	}
	switch {
	case m.saving:
		title, keys, note = theme.Accent.Render("Saving…"), nil, theme.Dim.Render("answer the UAC prompt if one opened")
	case m.outcome != nil:
		title, keys, note = theme.Accent.Render("Saved"), []key.Binding{keyDoneBack, keyDoneQuit}, ""
	case m.confirmSave:
		keys, note = []key.Binding{keySureYes, keySureNo}, theme.Err.Render("Save the changes marked ! as well? ")
	}
	if m.review.TotalLineCount() > m.review.Height() {
		keys = append([]key.Binding{keyScroll}, keys...)
	}
	inner := m.h - theme.Panel.GetVerticalFrameSize()
	body := title + "\n\n" + m.review.View()
	body += strings.Repeat("\n", max(inner-lipgloss.Height(body), 1)) + note + m.help.ShortHelpView(keys)
	return theme.Panel.BorderForeground(theme.ColorAccent).Width(m.w).Height(m.h).Render(body)
}

func (m Model) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.saving {
		return m, nil // nothing to do but wait
	}
	switch {
	case m.outcome != nil:
		switch {
		case key.Matches(msg, keyDoneQuit):
			return m, tea.Quit
		case key.Matches(msg, keyDoneBack):
			m.reviewing, m.outcome = false, nil
			return m, nil
		}
	case m.confirmSave:
		switch {
		case key.Matches(msg, keySureYes):
			return m.save()
		case key.Matches(msg, keySureNo):
			m.confirmSave = false
		}
		return m, nil
	case key.Matches(msg, keyReviewSave):
		if len(m.warnings()) > 0 {
			m.confirmSave = true
			return m, nil
		}
		return m.save()
	case key.Matches(msg, keyReviewBack):
		m.reviewing = false
		return m, nil
	}
	// Any other key may scroll.
	m.syncReview()
	var cmd tea.Cmd
	m.review, cmd = m.review.Update(msg)
	return m, cmd
}

// savedMsg brings back how each write went: an error per tab, nil when it was saved.
type savedMsg struct{ errs map[int]error }

// save writes every changed tab off the UI loop: writing Machine variables unelevated waits
// for UAC. The tabs are only read here; saved updates them when the answers are in.
func (m Model) save() (tea.Model, tea.Cmd) {
	jobs := map[int][]winenv.Change{}
	for i, t := range m.tabs {
		if s, d := dirty(t); d {
			jobs[i] = s.Changes()
		}
	}
	m.saving, m.confirmSave = true, false
	apply := m.opts.Apply
	return m, func() tea.Msg {
		errs := map[int]error{}
		for i := range len(m.tabs) { // in order, User first: a declined UAC prompt cannot hold it up
			if changes, ok := jobs[i]; ok {
				errs[i] = apply(changes)
			}
		}
		return savedMsg{errs}
	}
}

// saved reloads what was written; a tab that failed is marked and keeps its changes for
// another try.
func (m Model) saved(msg savedMsg) (tea.Model, tea.Cmd) {
	m.saving, m.outcome = false, []string{}
	for i, t := range m.tabs {
		err, tried := msg.errs[i]
		switch {
		case !tried:
		case err != nil:
			m.saveErr[i] = err
			m.outcome = append(m.outcome, theme.Err.Render("✗ ")+t.Label()+" not saved: "+err.Error(),
				theme.Dim.Render("  its changes are kept: s tries again"))
		default:
			m.saveErr[i] = nil
			t.Load()
			m.outcome = append(m.outcome, theme.OK.Render("✓ ")+t.Label()+" saved"+
				theme.Dim.Render(`  (the old values are in %LOCALAPPDATA%\enved)`))
		}
	}
	if m.opts.AfterSave != "" {
		m.outcome = append(m.outcome, "", theme.Dim.Render(m.opts.AfterSave))
	}
	m.syncReview()
	m.review.GotoTop()
	return m, nil
}
