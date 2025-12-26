package chainart

import (
	"strings"
	"testing"
)

func TestBuildChainArt(t *testing.T) {
	art := BuildChainArt()

	// Should contain 6 lines (broken link height)
	lines := strings.Split(art, "\n")
	if len(lines) != 6 {
		t.Errorf("expected 6 lines, got %d", len(lines))
	}

	// Should contain broken link characters
	if !strings.Contains(art, "\\│/") {
		t.Error("expected broken link crack characters")
	}
}

func TestBuildIntactChainArt(t *testing.T) {
	art := BuildIntactChainArt()

	// Should contain 4 lines (all links same height)
	lines := strings.Split(art, "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(lines))
	}

	// Should NOT contain broken link characters
	if strings.Contains(art, "\\│/") {
		t.Error("intact chain should not have broken link crack characters")
	}
	if strings.Contains(art, "/│\\") {
		t.Error("intact chain should not have broken link crack characters")
	}

	// Should contain standard link box characters
	if !strings.Contains(art, "╔═══════╗") {
		t.Error("expected standard link box top")
	}
	if !strings.Contains(art, "╚═══════╝") {
		t.Error("expected standard link box bottom")
	}

	// Should contain connectors
	if !strings.Contains(art, "═══") {
		t.Error("expected connector characters")
	}
}

func TestBuildIntactChainArt_FiveLinkStructure(t *testing.T) {
	art := BuildIntactChainArt()

	// Count link boundaries - should have 5 top corners and 5 bottom corners
	topCornerCount := strings.Count(art, "╔═══════╗")
	bottomCornerCount := strings.Count(art, "╚═══════╝")

	if topCornerCount != 5 {
		t.Errorf("expected 5 link tops (╔═══════╗), got %d", topCornerCount)
	}
	if bottomCornerCount != 5 {
		t.Errorf("expected 5 link bottoms (╚═══════╝), got %d", bottomCornerCount)
	}
}

func TestBuildIntactChainArt_HasConnectors(t *testing.T) {
	art := BuildIntactChainArt()
	lines := strings.Split(art, "\n")

	// Lines 1 and 2 (0-indexed) should have connectors between links
	// Each connector is "═══" and there are 4 connectors (between 5 links)
	for i, line := range lines {
		if i == 1 || i == 2 {
			// These lines should have connectors
			connectorCount := strings.Count(line, "═══")
			// Connectors appear between links, but "═══" also appears in box tops/bottoms
			// On middle lines, we expect connector pieces
			if connectorCount < 4 {
				t.Errorf("line %d expected at least 4 connector segments, got %d", i, connectorCount)
			}
		}
	}
}

func TestBuildIntactChainArt_ConsistentLineWidths(t *testing.T) {
	art := BuildIntactChainArt()
	lines := strings.Split(art, "\n")

	// All lines should have consistent visual width for proper alignment
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}

	// The first and last lines (top/bottom of boxes) should be similar width
	// This ensures proper visual alignment
	firstLineLen := len([]rune(lines[0]))
	lastLineLen := len([]rune(lines[3]))

	if firstLineLen != lastLineLen {
		t.Errorf("first line width (%d) != last line width (%d), alignment issue", firstLineLen, lastLineLen)
	}
}

func TestBuildProgressChainArt_NoFailure_AllPending(t *testing.T) {
	art := BuildProgressChainArt(0, -1)
	lines := strings.Split(art, "\n")

	// Should be 4 lines (intact chain - no failure)
	if len(lines) != 4 {
		t.Errorf("expected 4 lines for intact chain, got %d", len(lines))
	}

	// Should have 5 link tops
	topCount := strings.Count(art, "╔═══════╗")
	if topCount != 5 {
		t.Errorf("expected 5 link tops, got %d", topCount)
	}
}

func TestBuildProgressChainArt_NoFailure_AllComplete(t *testing.T) {
	art := BuildProgressChainArt(5, -1)
	lines := strings.Split(art, "\n")

	// Should be 4 lines (intact chain)
	if len(lines) != 4 {
		t.Errorf("expected 4 lines for intact chain, got %d", len(lines))
	}

	// Should have 5 link tops
	topCount := strings.Count(art, "╔═══════╗")
	if topCount != 5 {
		t.Errorf("expected 5 link tops, got %d", topCount)
	}
}

func TestBuildProgressChainArt_FailedAtFirstPhase(t *testing.T) {
	art := BuildProgressChainArt(0, 0) // Failed at first phase
	lines := strings.Split(art, "\n")

	// Should be 6 lines (has broken link which is taller)
	if len(lines) != 6 {
		t.Errorf("expected 6 lines with broken link, got %d", len(lines))
	}

	// Should contain broken link characters
	if !strings.Contains(art, "\\│/") {
		t.Error("expected broken link crack characters")
	}

	// Should have 4 regular link tops (broken link has different structure)
	topCount := strings.Count(art, "╔═══════╗")
	if topCount != 4 {
		t.Errorf("expected 4 regular link tops (1 broken), got %d", topCount)
	}
}

func TestBuildProgressChainArt_FailedAtMiddlePhase(t *testing.T) {
	art := BuildProgressChainArt(2, 2) // Failed at phase 2 (SpawningWorkers)
	lines := strings.Split(art, "\n")

	// Should be 6 lines (has broken link)
	if len(lines) != 6 {
		t.Errorf("expected 6 lines with broken link, got %d", len(lines))
	}

	// Should contain broken link characters
	if !strings.Contains(art, "\\│/") {
		t.Error("expected broken link crack characters")
	}
}

func TestBuildProgressChainArt_FailedAtLastPhase(t *testing.T) {
	art := BuildProgressChainArt(3, 3) // Failed at phase 3 (WorkersReady)
	lines := strings.Split(art, "\n")

	// Should be 6 lines (has broken link)
	if len(lines) != 6 {
		t.Errorf("expected 6 lines with broken link, got %d", len(lines))
	}

	// Should contain broken link characters
	if !strings.Contains(art, "\\│/") {
		t.Error("expected broken link crack characters")
	}
}
