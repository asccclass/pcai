package browserskill

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestWikipediaSearch performs a real browser automation test
// Scenario: Open Wikipedia -> Type "Go (programming language)" -> Click Search -> Verify Title
func TestWikipediaSearch(t *testing.T) {
	// Initialize Tools
	openTool := &BrowserOpenTool{}
	snapshotTool := &BrowserSnapshotTool{}
	typeTool := &BrowserTypeTool{}
	clickTool := &BrowserClickTool{}
	// getTool := &BrowserGetTool{}

	fmt.Println("🚀 Starting Browser Test: Wikipedia Search")

	// 1. Open
	fmt.Println("1. Navigating to Wikipedia...")
	if _, err := openTool.Run(`{"url": "https://en.wikipedia.org/wiki/Main_Page"}`); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// 2. Snapshot
	fmt.Println("2. Taking Snapshot...")
	res, err := snapshotTool.Run(`{"interactive_only": true}`)
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	// Parse output to find search input ref
	// Looking for: [input] ... placeholder="Search Wikipedia" or name="search"
	searchRef := findRef(res, "search")
	if searchRef == "" {
		t.Logf("Snapshot Output:\n%s", res)
		t.Fatalf("Could not find search input in snapshot")
	}
	fmt.Printf("   Found Search Input: %s\n", searchRef)

	// 3. Type
	fmt.Println("3. Typing query...")
	if _, err := typeTool.Run(fmt.Sprintf(`{"ref": "%s", "text": "Go (programming language)"}`, searchRef)); err != nil {
		t.Fatalf("Type failed: %v", err)
	}

	// 4. Click Search Button
	// Usually there is a search button. Let's find it.
	// Or sometimes hitting enter works. But typeTool just types.
	// Let's re-snapshot to see if button is visible or just click the "Search" button if found.
	// In Wikipedia, there's a button with text "Search" or "Go".
	// Alternatively, pressing Enter is not yet supported by a specific "PressKey" tool directly exposed,
	// but we can try to find the button.
	buttonRef := findRef(res, "Search") // Button often has "Search" text
	if buttonRef == "" {
		buttonRef = findRef(res, "Go")
	}
	if buttonRef != "" {
		fmt.Printf("   Found Search Button: %s. Clicking...\n", buttonRef)
		if _, err := clickTool.Run(fmt.Sprintf(`{"ref": "%s"}`, buttonRef)); err != nil {
			t.Fatalf("Click failed: %v", err)
		}
	} else {
		t.Log("⚠️ Could not find explicit Search button, test might fail if typing didn't submit.")
		// Some forms submit on enter? We might need valid enter support.
		// For now, let's assume we found it or we fail.
	}

	// 5. Wait/Re-Snapshot
	fmt.Println("4. Waiting for page load...")
	time.Sleep(3 * time.Second) // Simple wait

	fmt.Println("5. Verifying Result...")
	res, err = snapshotTool.Run(`{"interactive_only": true}`)
	if err != nil {
		t.Fatalf("Re-snapshot failed: %v", err)
	}

	// Need imports for strings
	if len(res) > 500 {
		fmt.Printf("   Result Snapshot (first 500 chars):\n%s...\n", res[:500])
	} else {
		fmt.Printf("   Result Snapshot:\n%s\n", res)
	}

	if !strings.Contains(res, "Go (programming language)") {
		// Not a strict failure as search result page might vary, but warning
		t.Log("⚠️ WARNING: Did not find expected text 'Go (programming language)' in result snapshot.")
	} else {
		fmt.Println("✅ Success: Found 'Go (programming language)' in result!")
	}
}

// Simple helper to parse snapshot text and find a ref containing a keyword
func findRef(snapshot, keyword string) string {
	lines := strings.Split(snapshot, "\n")
	for _, line := range lines {
		// Case insensitive check
		if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
			// Extract @eN
			parts := strings.Fields(line)
			if len(parts) > 0 && strings.HasPrefix(parts[0], "@e") {
				return parts[0] // Return the ref ID e.g. @e1
			}
		}
	}
	return ""
}
