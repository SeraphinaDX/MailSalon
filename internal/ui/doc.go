// Package uiapp contains MailSalon's terminal user interface and interaction state.
//
// App owns the current account, discovered folders, message list, preview state,
// search state, compose state, and gotui widgets. Run is a single event loop: it
// receives terminal events, routes them to the active mode (normal browsing,
// search, or compose), mutates application state, and redraws the interface.
// Keeping those transitions in one goroutine avoids a second synchronization
// model inside the UI.
//
// The compose editor needs a few MailSalon-specific workarounds for gotui. In
// particular, gotui currently reports pasted text as ordinary key events and
// its TextArea clips long logical lines instead of wrapping them. The compose
// handlers therefore keep Tab inside the body during editing and call the local
// wrapping helper after inserted text. Those behaviors are intentional and have
// regression tests; changing key dispatch should preserve them.
//
// Network mail operations are not implemented here. The UI reads/writes the
// local Maildir and invokes the transport package for configured receive/send
// commands, keeping protocol-specific synchronization outside the interface.
package uiapp
