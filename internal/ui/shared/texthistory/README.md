# texthistory

Package `texthistory` provides command history and draft preservation for text input fields in the xorchestrator TUI.

## Overview

This package implements two key features inspired by readline and claudex-cli's InputBox:

1. **History Navigation**: Ring buffer-based command history with Previous/Next navigation
2. **Draft Preservation**: Per-context draft storage with automatic save/restore

Both components are thread-safe and designed for concurrent use in a BubbleTea application.

## Components

### HistoryManager

Manages a ring buffer of history entries with navigation support.

**Features:**
- Configurable maximum size (default: 100 entries, max: 10,000)
- Duplicate detection (consecutive identical entries not added)
- Empty string filtering
- Thread-safe concurrent access
- O(1) add, previous, and next operations

**State Machine:**

```
    [Idle]
      |
      | Previous()
      ↓
[Navigating] ←→ Previous/Next
      |
      | Next() at newest OR Reset() OR Add()
      ↓
    [Idle]
```

**Example:**
```go
h := texthistory.NewHistoryManager(100)

// Add entries
h.Add("git status")
h.Add("git commit -m 'fix'")

// Navigate backward
entry := h.Previous() // Returns "git commit -m 'fix'"
entry = h.Previous()  // Returns "git status"

// Navigate forward
entry = h.Next()      // Returns "git commit -m 'fix'"
entry = h.Next()      // Returns "" and exits navigation

// Check state
if h.IsNavigating() {
    fmt.Println("Currently at:", h.Current())
}
```

### DraftStore

Manages per-context draft preservation for text fields.

**Features:**
- Context-based isolation (formID:fieldID:sessionID)
- Thread-safe concurrent access
- O(1) save and load operations
- Automatic cleanup on empty save

**Context ID Format:**
```
"{formID}:{fieldID}:{sessionID}"

Examples:
- "issueeditor:description:session-abc123"
- "formmodal:title:session-xyz789"
```

**Example:**
```go
d := texthistory.NewDraftStore()

// Save draft
contextID := "issueeditor:description:session-123"
d.Save(contextID, "Work in progress...")

// Load draft later
if draft, ok := d.Load(contextID); ok {
    fmt.Println("Restored draft:", draft)
}

// Clear specific draft
d.Clear(contextID)

// Clear all drafts
d.ClearAll()
```

## Integration Patterns

### With formmodal

```go
// In app initialization (cmd/root.go)
historyMgr := texthistory.NewHistoryManager(100)
draftStore := texthistory.NewDraftStore()

// Pass to formmodal
formmodal.SetHistoryProviders(historyMgr, draftStore)

// In formmodal field config
field := formmodal.FieldConfig{
    Type:          formmodal.FieldTypeText,
    EnableHistory: true,  // Enable history for this field
    // ...
}
```

### State Management in BubbleTea

```go
type Model struct {
    historyMgr *texthistory.HistoryManager
    draftStore *texthistory.DraftStore
    contextID  string
    text       string
    inHistory  bool
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "up":
            if !m.historyMgr.IsNavigating() {
                // Save current text as draft before entering history
                m.draftStore.Save(m.contextID, m.text)
            }
            m.text = m.historyMgr.Previous()
            m.inHistory = true

        case "down":
            if m.historyMgr.IsNavigating() {
                next := m.historyMgr.Next()
                if next == "" {
                    // Exited history - restore draft
                    if draft, ok := m.draftStore.Load(m.contextID); ok {
                        m.text = draft
                    }
                    m.inHistory = false
                } else {
                    m.text = next
                }
            }

        case "esc":
            if m.inHistory {
                // Cancel history navigation - restore draft
                if draft, ok := m.draftStore.Load(m.contextID); ok {
                    m.text = draft
                }
                m.historyMgr.Reset()
                m.inHistory = false
            }

        case "enter":
            // Submit - add to history
            m.historyMgr.Add(m.text)
            m.draftStore.Clear(m.contextID)
            m.inHistory = false
        }
    }
    return m, nil
}
```

## Performance Characteristics

| Operation | Time Complexity | Notes |
|-----------|----------------|-------|
| Add | O(1) amortized | O(n) worst case when buffer full (shift required) |
| Previous | O(1) | Direct array access |
| Next | O(1) | Direct array access |
| Save | O(1) | Map insert |
| Load | O(1) | Map lookup |
| Clear | O(1) | Map delete |

**Memory Usage:**
- HistoryManager: ~24 bytes + (entries × avg_string_size)
- DraftStore: ~24 bytes + (contexts × avg_draft_size)

**Benchmarks:**
- Add: < 100ns per operation
- Previous/Next: < 50ns per operation
- Save/Load: < 100ns per operation

## Thread Safety

All operations are thread-safe through the use of sync.RWMutex:

- Read operations (Previous, Next, Current, Load, Has, Len) use RLock
- Write operations (Add, Save, Clear) use Lock

This allows multiple concurrent readers or a single writer.

## Testing

The package includes comprehensive test coverage:

- **history_test.go**: 50+ tests covering navigation, edge cases, Unicode, concurrency
- **drafts_test.go**: 30+ tests covering isolation, persistence, edge cases, concurrency

Run tests:
```bash
cd internal/ui/shared/texthistory
go test -v
go test -race  # Check for race conditions
go test -bench=.  # Run benchmarks
```

## Configuration

### Constants

```go
const (
    DefaultMaxHistory = 100    // Default history size
    MaxHistorySize    = 10000  // Maximum allowed size
)
```

### Customization

```go
// Custom history size
h := texthistory.NewHistoryManager(500)

// Unbounded draft storage (limited only by memory)
d := texthistory.NewDraftStore()
```

## Design Decisions

### Why Ring Buffer for History?

Ring buffers provide O(1) add with automatic oldest-entry eviction, avoiding unbounded memory growth while maintaining recent command history.

### Why Context-Based Drafts?

Context isolation prevents draft pollution across different forms, fields, and user sessions. The format `formID:fieldID:sessionID` ensures proper scoping.

### Why Thread-Safe by Default?

BubbleTea applications may spawn concurrent commands (tea.Cmd) that access shared state. Thread safety prevents race conditions without requiring callers to implement locking.

### Why Not Persist to Disk?

The MVP implementation uses in-memory storage for simplicity and performance. Disk persistence can be added in Phase 2 using JSON serialization of HistoryManager.Entries() and DraftStore contents.

## Future Enhancements

- [ ] Disk persistence to `~/.config/xorchestrator/history.json`
- [ ] History search/filtering
- [ ] Configurable duplicate detection (fuzzy matching)
- [ ] LRU eviction for draft store
- [ ] History compression for long-running sessions
- [ ] Cross-session history synchronization

## References

- claudex-cli InputBox: [InputBox.tsx](https://github.com/example/claudex-cli/blob/main/src/ui/components/InputBox.tsx)
- BubbleTea Key Handling: https://github.com/charmbracelet/bubbletea
- Go Concurrency Patterns: https://go.dev/blog/context
