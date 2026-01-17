package texthistory

import (
	"sync"
)

// DraftStore manages per-context draft preservation for text input fields.
// It provides thread-safe storage and retrieval of draft text keyed by context ID.
//
// Context IDs should follow the format: "{formID}:{fieldID}:{sessionID}"
// This ensures isolation between different forms, fields, and sessions.
type DraftStore struct {
	drafts map[string]string // Map of contextID -> draft text
	mu     sync.RWMutex
}

// NewDraftStore creates a new draft store.
func NewDraftStore() *DraftStore {
	return &DraftStore{
		drafts: make(map[string]string),
	}
}

// Save stores a draft for the given context ID.
// If text is empty, the draft is removed (equivalent to Clear).
func (d *DraftStore) Save(contextID, text string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if text == "" {
		delete(d.drafts, contextID)
	} else {
		d.drafts[contextID] = text
	}
}

// Load retrieves a draft for the given context ID.
// Returns the draft text and true if found, empty string and false if not found.
func (d *DraftStore) Load(contextID string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	text, ok := d.drafts[contextID]
	return text, ok
}

// Clear removes the draft for the given context ID.
func (d *DraftStore) Clear(contextID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.drafts, contextID)
}

// ClearAll removes all drafts from the store.
func (d *DraftStore) ClearAll() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.drafts = make(map[string]string)
}

// Has returns true if a draft exists for the given context ID.
func (d *DraftStore) Has(contextID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, ok := d.drafts[contextID]
	return ok
}

// Len returns the number of drafts currently stored.
func (d *DraftStore) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.drafts)
}

// ContextIDs returns a slice of all context IDs that have stored drafts.
// This is safe for concurrent use as it returns a copy.
func (d *DraftStore) ContextIDs() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids := make([]string, 0, len(d.drafts))
	for id := range d.drafts {
		ids = append(ids, id)
	}
	return ids
}
