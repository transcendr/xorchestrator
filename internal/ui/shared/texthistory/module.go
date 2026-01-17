// Package texthistory provides command history and draft preservation
// for text input fields in the xorchestrator TUI.
//
// This package is designed to integrate with formmodal and other text input
// components, providing a seamless history navigation and draft management
// experience similar to readline or claudex-cli's InputBox.
//
// # Architecture
//
// The package consists of two main components:
//
//   - HistoryManager: Manages a ring buffer of command history entries with
//     Previous/Next navigation
//   - DraftStore: Manages per-context draft preservation with contextID-based
//     isolation
//
// Both components are thread-safe and can be used concurrently from multiple
// goroutines.
//
// # Usage
//
// Typical usage involves creating singleton instances at application startup
// and passing them to components that need history/draft support:
//
//	// In app initialization (cmd/root.go)
//	historyMgr := texthistory.NewHistoryManager(100)
//	draftStore := texthistory.NewDraftStore()
//
//	// Pass to formmodal or other components
//	formmodal.SetHistoryProviders(historyMgr, draftStore)
//
// # Context ID Format
//
// Draft context IDs should follow the format: "{formID}:{fieldID}:{sessionID}"
// This ensures proper isolation between different forms, fields, and sessions.
//
// Example: "issueeditor:description:session-abc123"
package texthistory
