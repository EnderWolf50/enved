// Package frame is the screen enved and pathed share: a sidebar of tabs and a panel with
// the selected tab's content; a review of every change before saving; the save itself, off
// the UI loop because UAC may be asking; and a quit dialog that only asks about unsaved
// changes. A tab supplies its content and its changes; the frame does the rest.
package frame

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EnderWolf50/enved/listedit"
	"github.com/EnderWolf50/enved/theme"
	"github.com/EnderWolf50/enved/winenv"
)

// Event is what a key in a tab asks of the frame; the same as the list editor's.
type Event = listedit.Event

const (
	None   = listedit.None
	Back   = listedit.Back   // back to the sidebar
	Save   = listedit.Save   // review and save
	Reload = listedit.Reload // read the tab again
)

// Tab is one entry of the sidebar and what the panel shows for it.
type Tab interface {
	Name() string  // in the sidebar: "User"
	Label() string // in the review and the outcome: "User PATH", "User variables"
	Count() string // in the sidebar, after the name
	Err() error    // it could not be read
	ReadOnly() bool
	NeedsAdmin() bool // saving it asks for admin (UAC)

	Dirty() bool
	Pending() int             // the number of changes, for the quit question
	Changes() []winenv.Change // what saving writes
	Review() []string         // the changes, one line each, for the review
	Warnings() []string       // changes to confirm a second time before saving
	Load()                    // read it again, dropping every change
	Update(tea.Msg) (tea.Cmd, Event)

	Resize(w, h int) // the body's size
	Focus(bool)      // whether the tab has the keys
	Heading() string // the panel's first line
	Body() string    // the rest of the panel
	Overlay() string // a box drawn over the screen, or ""
	Status() string  // the tab's one-off note, or ""
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
}

// New is the screen over tabs, with the sidebar on tab on; open puts the keys in its body.
func New(opts Options, tabs []Tab, on int, open bool) Model {
	m := Model{opts: opts, tabs: tabs, saveErr: make([]error, len(tabs)), on: on}
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
		return m, nil
	case savedMsg:
		return m.saved(msg)
	case tea.KeyPressMsg:
		m.status = ""
		if msg.String() == "ctrl+c" {
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
	return slices.ContainsFunc(m.tabs, func(t Tab) bool { return t.Dirty() })
}

// The sidebar picks a tab; enter hands the keys to its body.
func (m Model) updateSide(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		if m.anyDirty() {
			m.confirmQuit = true
			return m, nil
		}
		return m, tea.Quit
	case "s":
		return m.startReview()
	case "r":
		return m.reload()
	case "up", "k":
		m.on = max(m.on-1, 0)
	case "down", "j":
		m.on = min(m.on+1, len(m.tabs)-1)
	case "enter", "right", "l":
		m.inBody = true
		m.tab().Focus(true)
	}
	return m, nil
}

// reload reads the selected tab again, unless it has changes that would be lost.
func (m Model) reload() (tea.Model, tea.Cmd) {
	if m.tab().Dirty() {
		m.status = "unsaved changes: u undoes them first"
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
		count := t.Count()
		switch {
		case t.Err() != nil:
			count = theme.Err.Render("!")
		case m.saveErr[i] != nil:
			count = theme.Err.Render("!") + " " + count
		case t.ReadOnly():
			count += " ro"
		case t.NeedsAdmin():
			count += " uac"
		}
		if t.Dirty() {
			count += " *"
		}
		// The panel's border and padding take 4 cells, the "▌ " mark 2 and the name 10.
		label := fmt.Sprintf("%-9s %s", t.Name(), lipgloss.PlaceHorizontal(sideW-16, lipgloss.Right, count))
		if i == m.on {
			side = append(side, theme.Accent.Render("▌ "+label))
		} else {
			side = append(side, theme.Dim.Render("  "+label))
		}
	}

	focused := theme.Panel.BorderForeground(theme.ColorAccent)
	sideStyle, bodyStyle := theme.Panel, focused
	if !m.inBody {
		sideStyle, bodyStyle = focused, theme.Panel
	}
	// While it has focus, the sidebar keeps its keys at the bottom. (The filler string adds
	// one line more than its newlines.)
	if !m.inBody {
		foot := []string{"↑/k      up", "↓/j      down", "→/enter  open", "r        reload", "s        save", "esc/q    quit"}
		inner := m.h - sideStyle.GetVerticalFrameSize()
		side = append(side, strings.Repeat("\n", max(inner-len(side)-len(foot)-1, 0)))
		side = append(side, theme.Dim.Render(strings.Join(foot, "\n")))
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
	switch {
	case t.Err() != nil:
		heading += theme.Err.Render(" · could not be read: " + t.Err().Error())
	case m.saveErr[m.on] != nil:
		heading += theme.Err.Render(" · not saved: " + m.saveErr[m.on].Error())
	case t.ReadOnly():
		heading += theme.Dim.Render(" · read-only")
	case t.NeedsAdmin():
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
		n += t.Pending()
	}
	return theme.Modal.Render(lipgloss.JoinVertical(lipgloss.Center,
		theme.Err.Bold(true).Render("Quit without saving?"), "", theme.Plural(n, "change", "changes")+" will be lost.", "",
		theme.Dim.Render("y/q quit · n/esc stay")))
}

// The quit dialog only answers the question; every other key is ignored while it is open.
func (m Model) updateQuit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "q":
		return m, tea.Quit
	case "n", "esc":
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
	return m, nil
}

func (m Model) warnings() []string {
	var all []string
	for _, t := range m.tabs {
		if t.Dirty() {
			all = append(all, t.Warnings()...)
		}
	}
	return all
}

// reviewLines lists every change, tab by tab, each tab's warnings first.
func (m Model) reviewLines(width int) []string {
	var lines []string
	for _, t := range m.tabs {
		if !t.Dirty() {
			continue
		}
		head := theme.Accent.Render(t.Label())
		if t.NeedsAdmin() {
			head += theme.Dim.Render("  saving it asks for admin: UAC will prompt")
		}
		lines = append(lines, head)
		for _, w := range t.Warnings() {
			lines = append(lines, theme.Err.Render("  ! "+w))
		}
		lines = append(lines, t.Review()...)
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return lines
}

func (m Model) viewReview() string {
	width := m.w - theme.Panel.GetHorizontalFrameSize()
	title := theme.Accent.Render("Review") + theme.Dim.Render(" · what saving will write")
	lines, help := m.reviewLines(width), "enter/s save · esc/q back to the list"
	if len(m.warnings()) > 0 {
		help = "enter/s save (it asks again: the changes marked ! need a second look) · esc/q back"
	}
	switch {
	case m.saving:
		title, help = theme.Accent.Render("Saving…"), "answer the UAC prompt if one opened"
	case m.outcome != nil:
		title, lines, help = theme.Accent.Render("Saved"), m.outcome, "enter/esc back to the list · q quit"
	case m.confirmSave:
		help = theme.Err.Render("Save the changes marked ! as well? enter/y save · esc/n back")
	}
	room := max(m.h-theme.Panel.GetVerticalFrameSize()-4, 1)
	if len(lines) > room {
		more := len(lines) - room + 1
		lines = append(lines[:room-1], theme.Dim.Render(fmt.Sprintf("… and %d more lines", more)))
	}
	body := title + "\n\n" + strings.Join(lines, "\n")
	inner := m.h - theme.Panel.GetVerticalFrameSize()
	body += strings.Repeat("\n", max(inner-lipgloss.Height(body), 1)) + theme.Dim.Render(help)
	return theme.Panel.BorderForeground(theme.ColorAccent).Width(m.w).Height(m.h).Render(body)
}

func (m Model) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.saving {
		return m, nil // nothing to do but wait
	}
	if m.outcome != nil {
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "enter", "esc":
			m.reviewing, m.outcome = false, nil
		}
		return m, nil
	}
	if m.confirmSave {
		switch msg.String() {
		case "enter", "y":
			return m.save()
		case "esc", "n", "q":
			m.confirmSave = false
		}
		return m, nil
	}
	switch msg.String() {
	case "enter", "s":
		if len(m.warnings()) > 0 {
			m.confirmSave = true
			return m, nil
		}
		return m.save()
	case "esc", "q", "left", "h":
		m.reviewing = false
	}
	return m, nil
}

// savedMsg brings back how each write went: an error per tab, nil when it was saved.
type savedMsg struct{ errs map[int]error }

// save writes every changed tab off the UI loop: writing Machine variables unelevated waits
// for UAC. The tabs are only read here; saved updates them when the answers are in.
func (m Model) save() (tea.Model, tea.Cmd) {
	jobs := map[int][]winenv.Change{}
	for i, t := range m.tabs {
		if t.Dirty() {
			jobs[i] = t.Changes()
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
	return m, nil
}
