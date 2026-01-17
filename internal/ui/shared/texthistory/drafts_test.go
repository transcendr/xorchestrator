package texthistory

import (
	"strings"
	"sync"
	"testing"
)

func TestNewDraftStore(t *testing.T) {
	d := NewDraftStore()
	if d == nil {
		t.Fatal("expected non-nil DraftStore")
	}
	if d.Len() != 0 {
		t.Errorf("Len() = %d, want 0", d.Len())
	}
}

func TestSaveAndLoad(t *testing.T) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"
	text := "draft content"

	d.Save(contextID, text)

	got, ok := d.Load(contextID)
	if !ok {
		t.Fatal("Load() returned false, want true")
	}
	if got != text {
		t.Errorf("Load() = %q, want %q", got, text)
	}
}

func TestLoadNonExistent(t *testing.T) {
	d := NewDraftStore()
	got, ok := d.Load("nonexistent")
	if ok {
		t.Error("Load() returned true for non-existent, want false")
	}
	if got != "" {
		t.Errorf("Load() = %q, want %q", got, "")
	}
}

func TestSaveEmpty(t *testing.T) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"

	// Save non-empty first
	d.Save(contextID, "content")
	if d.Len() != 1 {
		t.Fatal("Len() should be 1 after Save")
	}

	// Save empty should remove it
	d.Save(contextID, "")
	if d.Len() != 0 {
		t.Errorf("Len() = %d after saving empty, want 0", d.Len())
	}

	_, ok := d.Load(contextID)
	if ok {
		t.Error("Load() returned true after saving empty, want false")
	}
}

func TestSaveOverwrite(t *testing.T) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"

	d.Save(contextID, "first")
	d.Save(contextID, "second")

	got, ok := d.Load(contextID)
	if !ok {
		t.Fatal("Load() returned false, want true")
	}
	if got != "second" {
		t.Errorf("Load() = %q, want %q", got, "second")
	}
	if d.Len() != 1 {
		t.Errorf("Len() = %d, want 1", d.Len())
	}
}

func TestMultipleContexts(t *testing.T) {
	d := NewDraftStore()

	contexts := map[string]string{
		"form1:field1:session1": "draft1",
		"form1:field2:session1": "draft2",
		"form2:field1:session1": "draft3",
		"form1:field1:session2": "draft4",
	}

	for ctx, txt := range contexts {
		d.Save(ctx, txt)
	}

	if d.Len() != len(contexts) {
		t.Errorf("Len() = %d, want %d", d.Len(), len(contexts))
	}

	// Verify all can be loaded correctly
	for ctx, want := range contexts {
		got, ok := d.Load(ctx)
		if !ok {
			t.Errorf("Load(%q) returned false, want true", ctx)
		}
		if got != want {
			t.Errorf("Load(%q) = %q, want %q", ctx, got, want)
		}
	}
}

func TestDraftClear(t *testing.T) {
	d := NewDraftStore()
	d.Save("ctx1", "draft1")
	d.Save("ctx2", "draft2")

	d.Clear("ctx1")

	if _, ok := d.Load("ctx1"); ok {
		t.Error("Load(ctx1) returned true after Clear(), want false")
	}
	if _, ok := d.Load("ctx2"); !ok {
		t.Error("Load(ctx2) returned false, want true (should not be cleared)")
	}
	if d.Len() != 1 {
		t.Errorf("Len() = %d after Clear(), want 1", d.Len())
	}
}

func TestDraftClearAll(t *testing.T) {
	d := NewDraftStore()
	d.Save("ctx1", "draft1")
	d.Save("ctx2", "draft2")
	d.Save("ctx3", "draft3")

	d.ClearAll()

	if d.Len() != 0 {
		t.Errorf("Len() = %d after ClearAll(), want 0", d.Len())
	}
	if _, ok := d.Load("ctx1"); ok {
		t.Error("Load(ctx1) returned true after ClearAll(), want false")
	}
}

func TestHas(t *testing.T) {
	d := NewDraftStore()

	if d.Has("ctx1") {
		t.Error("Has(ctx1) = true initially, want false")
	}

	d.Save("ctx1", "draft")

	if !d.Has("ctx1") {
		t.Error("Has(ctx1) = false after Save(), want true")
	}

	d.Clear("ctx1")

	if d.Has("ctx1") {
		t.Error("Has(ctx1) = true after Clear(), want false")
	}
}

func TestContextIDs(t *testing.T) {
	d := NewDraftStore()

	ids := d.ContextIDs()
	if len(ids) != 0 {
		t.Errorf("ContextIDs() length = %d initially, want 0", len(ids))
	}

	d.Save("ctx1", "draft1")
	d.Save("ctx2", "draft2")
	d.Save("ctx3", "draft3")

	ids = d.ContextIDs()
	if len(ids) != 3 {
		t.Errorf("ContextIDs() length = %d, want 3", len(ids))
	}

	// Verify all context IDs are present
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	for _, want := range []string{"ctx1", "ctx2", "ctx3"} {
		if !idMap[want] {
			t.Errorf("ContextIDs() missing %q", want)
		}
	}
}

// Test Context Isolation

func TestContextIsolation(t *testing.T) {
	d := NewDraftStore()

	// Different forms, same field and session
	d.Save("formA:field1:session1", "draftA")
	d.Save("formB:field1:session1", "draftB")

	gotA, okA := d.Load("formA:field1:session1")
	gotB, okB := d.Load("formB:field1:session1")

	if !okA || !okB {
		t.Fatal("Load() returned false, want true for both")
	}
	if gotA != "draftA" {
		t.Errorf("Load(formA) = %q, want %q", gotA, "draftA")
	}
	if gotB != "draftB" {
		t.Errorf("Load(formB) = %q, want %q", gotB, "draftB")
	}
}

func TestSessionIsolation(t *testing.T) {
	d := NewDraftStore()

	// Same form and field, different sessions
	d.Save("form1:field1:session1", "draft1")
	d.Save("form1:field1:session2", "draft2")

	got1, ok1 := d.Load("form1:field1:session1")
	got2, ok2 := d.Load("form1:field1:session2")

	if !ok1 || !ok2 {
		t.Fatal("Load() returned false, want true for both")
	}
	if got1 != "draft1" {
		t.Errorf("Load(session1) = %q, want %q", got1, "draft1")
	}
	if got2 != "draft2" {
		t.Errorf("Load(session2) = %q, want %q", got2, "draft2")
	}
}

// Test Edge Cases

func TestUnicodeInDraft(t *testing.T) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"
	draft := "Hello 👋 こんにちは 世界 🌍"

	d.Save(contextID, draft)

	got, ok := d.Load(contextID)
	if !ok {
		t.Fatal("Load() returned false, want true")
	}
	if got != draft {
		t.Errorf("Load() = %q, want %q", got, draft)
	}
}

func TestLargeDraft(t *testing.T) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"
	draft := strings.Repeat("a", 1000000) // 1M chars

	d.Save(contextID, draft)

	got, ok := d.Load(contextID)
	if !ok {
		t.Fatal("Load() returned false, want true")
	}
	if len(got) != len(draft) {
		t.Errorf("Load() length = %d, want %d", len(got), len(draft))
	}
}

func TestManyContexts(t *testing.T) {
	d := NewDraftStore()

	// Save many contexts (start from 1 to avoid empty string at i=0)
	for i := 1; i <= 1000; i++ {
		contextID := strings.Repeat("a", i)
		d.Save(contextID, strings.Repeat("b", i))
	}

	if d.Len() != 1000 {
		t.Errorf("Len() = %d, want 1000", d.Len())
	}

	// Verify random access works
	testCtx := strings.Repeat("a", 500)
	got, ok := d.Load(testCtx)
	if !ok {
		t.Error("Load() returned false for context 500, want true")
	}
	if len(got) != 500 {
		t.Errorf("Load() length = %d, want 500", len(got))
	}
}

// Test Concurrency

func TestConcurrentSaveLoad(t *testing.T) {
	d := NewDraftStore()
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				contextID := strings.Repeat("a", id*100+j)
				d.Save(contextID, strings.Repeat("b", j))
			}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				contextID := strings.Repeat("a", id*100+j)
				d.Load(contextID)
				d.Has(contextID)
			}
		}(i)
	}

	wg.Wait()
	// Test passes if no race condition detected
}

func TestConcurrentClear(t *testing.T) {
	d := NewDraftStore()

	// Populate
	for i := 0; i < 100; i++ {
		d.Save(strings.Repeat("a", i), strings.Repeat("b", i))
	}

	var wg sync.WaitGroup

	// Multiple goroutines clearing different contexts
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				contextID := strings.Repeat("a", id*10+j)
				d.Clear(contextID)
			}
		}(i)
	}

	wg.Wait()

	if d.Len() != 0 {
		t.Errorf("Len() = %d after clearing all, want 0", d.Len())
	}
}

// Benchmark tests

func BenchmarkSave(b *testing.B) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"
	draft := "benchmark draft content"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Save(contextID, draft)
	}
}

func BenchmarkLoad(b *testing.B) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"
	d.Save(contextID, "benchmark draft content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Load(contextID)
	}
}

func BenchmarkHas(b *testing.B) {
	d := NewDraftStore()
	contextID := "form1:field1:session1"
	d.Save(contextID, "benchmark draft content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Has(contextID)
	}
}

func BenchmarkSaveLoadMany(b *testing.B) {
	d := NewDraftStore()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		contextID := strings.Repeat("a", i%1000)
		d.Save(contextID, strings.Repeat("b", i%100))
		d.Load(contextID)
	}
}
