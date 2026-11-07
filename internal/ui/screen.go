package ui

import tea "charm.land/bubbletea/v2"

// Screen is one full-window view with its own state and keys. The root model
// keeps screens on a stack, and an overlay is a Screen drawn above the stack.
type Screen interface {
	Init() tea.Cmd
	// Update handles a message and returns the screen to keep, which may be a new one.
	Update(msg tea.Msg) (Screen, tea.Cmd)
	// View is the screen's text for the width and height given by the last WindowSizeMsg.
	View() string
}

// PushMsg opens a screen above the current one; esc comes back.
type PushMsg struct{ Screen Screen }

// PopMsg closes the current screen.
type PopMsg struct{}

// OpenMsg opens an overlay: it takes every key until it is closed.
type OpenMsg struct{ Overlay Screen }

// CloseMsg closes the top overlay.
type CloseMsg struct{}

// Push, Pop, Open and Close are commands that send the matching message.
func Push(s Screen) tea.Cmd { return func() tea.Msg { return PushMsg{s} } }
func Pop() tea.Cmd          { return func() tea.Msg { return PopMsg{} } }
func Open(o Screen) tea.Cmd { return func() tea.Msg { return OpenMsg{o} } }
func Close() tea.Cmd        { return func() tea.Msg { return CloseMsg{} } }

// Capturer is a screen that is taking text, such as a filter box. While it
// reports true, esc and the other keys the root usually keeps go to it.
type Capturer interface{ Capturing() bool }
