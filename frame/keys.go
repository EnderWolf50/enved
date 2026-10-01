package frame

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
)

// The frame's keys: the sidebar's, the quit question's and the review's. A tab has its own.
var (
	keyForceQuit = key.NewBinding(key.WithKeys("ctrl+c"))

	// The sidebar.
	keyUp     = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown   = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyOpen   = key.NewBinding(key.WithKeys("enter", "right", "l"), key.WithHelp("→/enter", "open"))
	keyReload = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload"))
	keySave   = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save"))
	keyQuit   = key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "quit"))

	// The quit question.
	keyQuitYes = key.NewBinding(key.WithKeys("y", "q"), key.WithHelp("y/q", "quit"))
	keyQuitNo  = key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "stay"))

	// The review, the second question about warnings, and the outcome of a save.
	keyReviewSave = key.NewBinding(key.WithKeys("enter", "s"), key.WithHelp("enter/s", "save"))
	keyReviewBack = key.NewBinding(key.WithKeys("esc", "q", "left", "h"), key.WithHelp("esc/q", "back to the list"))
	keySureYes    = key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("enter/y", "save"))
	keySureNo     = key.NewBinding(key.WithKeys("esc", "n", "q"), key.WithHelp("esc/n", "back"))
	keyDoneBack   = key.NewBinding(key.WithKeys("enter", "esc"), key.WithHelp("enter/esc", "back to the list"))
	keyDoneQuit   = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	keyScroll     = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓ pgup/pgdn", "scroll"))
)

// newReview is the review's scrolling area: up, down and the page keys; left and h are the
// review's way back, not a sideways scroll.
func newReview() viewport.Model {
	v := viewport.New()
	v.KeyMap.Left.SetEnabled(false)
	v.KeyMap.Right.SetEnabled(false)
	return v
}
