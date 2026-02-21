package browser

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// BrowserManager handles persistent browser sessions using Playwright
type BrowserManager struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	context playwright.BrowserContext
	page    playwright.Page

	mu        sync.Mutex
	refs      map[string]playwright.Locator
	lastRefID int
}

var (
	instance *BrowserManager
	once     sync.Once
)

// GetManager returns the singleton instance
func GetManager() *BrowserManager {
	once.Do(func() {
		instance = &BrowserManager{
			refs: make(map[string]playwright.Locator),
		}
	})
	return instance
}

// EnsureContext makes sure a Playwright browser & page are running
func (m *BrowserManager) EnsureContext() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.page != nil {
		// Check liveness: if it's closed, we'll recreate
		if !m.page.IsClosed() {
			return nil
		}
		m.cleanUp()
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start Playwright: %w", err)
	}
	m.pw = pw

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true), // Headless for server env
		Args: []string{
			"--disable-gpu",
			"--no-sandbox",
		},
	})
	if err != nil {
		m.cleanUp()
		return fmt.Errorf("could not launch Chromium: %w", err)
	}
	m.browser = browser

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
	})
	if err != nil {
		m.cleanUp()
		return fmt.Errorf("could not create context: %w", err)
	}
	m.context = context

	page, err := context.NewPage()
	if err != nil {
		m.cleanUp()
		return fmt.Errorf("could not create page: %w", err)
	}
	m.page = page

	return nil
}

// Navigate opens a URL
func (m *BrowserManager) Navigate(url string) error {
	if err := m.EnsureContext(); err != nil {
		return err
	}

	// Determine if file path or URL
	if !strings.HasPrefix(url, "http") && !strings.HasPrefix(url, "file://") {
		url = "https://" + url
	}

	// Wait until network is mostly idle to ensure dynamic content loads
	_, err := m.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000), // 30s timeout
	})
	return err
}

// Snapshot parses the DOM and returns a list of interactive elements with refs
func (m *BrowserManager) Snapshot(interactiveOnly bool) (string, error) {
	if err := m.EnsureContext(); err != nil {
		return "", err
	}

	m.mu.Lock()
	m.refs = make(map[string]playwright.Locator) // Clear old refs
	m.lastRefID = 0
	m.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Page URL: %s\n\n", m.page.URL()))

	// Use Playwright's getByRole to find interactive elements
	// To replicate OpenClaw's robust ARIA snapshot, we search for common interactive roles.
	rolesToFind := []*playwright.AriaRole{
		playwright.AriaRoleButton,
		playwright.AriaRoleLink,
		playwright.AriaRoleTextbox,
		playwright.AriaRoleCheckbox,
		playwright.AriaRoleRadio,
		playwright.AriaRoleCombobox,
		playwright.AriaRoleSearchbox,
		playwright.AriaRoleMenuitem,
		playwright.AriaRoleTab,
	}

	if !interactiveOnly {
		rolesToFind = append(rolesToFind,
			playwright.AriaRoleHeading,
			playwright.AriaRoleListitem,
			playwright.AriaRoleArticle,
		)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, rolePtr := range rolesToFind {
		if rolePtr == nil {
			continue
		}
		role := *rolePtr
		loc := m.page.GetByRole(role)

		// Wait a tiny bit for stability (optional, page is usually idle here)

		numElements, err := loc.Count()
		if err != nil {
			continue // Skip if error counting
		}

		for i := 0; i < numElements; i++ {
			nthLoc := loc.Nth(i)

			// Try to ensure it's visible before mapping it
			visible, err := nthLoc.IsVisible()
			if err != nil || !visible {
				continue
			}

			// Get accessible name or text content
			name := ""
			textContent, err := nthLoc.TextContent()
			if err == nil {
				name = strings.TrimSpace(textContent)
			}

			// Get placeholder or value for inputs if empty name
			if name == "" {
				if placeholder, err := nthLoc.GetAttribute("placeholder"); err == nil && placeholder != "" {
					name = placeholder
				} else if val, err := nthLoc.InputValue(); err == nil && val != "" {
					// InputValue works for inputs/textareas
					name = val
				} else if ariaLabel, err := nthLoc.GetAttribute("aria-label"); err == nil && ariaLabel != "" {
					name = ariaLabel
				}
			}

			// Squeeze whitespace
			name = strings.Join(strings.Fields(name), " ")
			if len(name) > 50 {
				name = name[:47] + "..."
			}

			m.lastRefID++
			ref := fmt.Sprintf("@e%d", m.lastRefID)
			m.refs[ref] = nthLoc

			if name != "" {
				sb.WriteString(fmt.Sprintf("- %s %q [ref=%s]\n", string(role), name, ref))
			} else {
				sb.WriteString(fmt.Sprintf("- %s [ref=%s]\n", string(role), ref))
			}
			count++
			if count > 200 {
				break
			}
		}
		if count > 200 {
			sb.WriteString("... (truncated to 200 elements) ...\n")
			break
		}
	}

	if count == 0 {
		return "No interactive elements found.", nil
	}

	return sb.String(), nil
}

// Click Ref
func (m *BrowserManager) Click(ref string) error {
	m.mu.Lock()
	loc, ok := m.refs[ref]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("ref %s not found. Page might have reloaded. Please snapshot again", ref)
	}

	// Playwright auto-scrolls into view and waits for actionability
	return loc.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(5000), // 5s timeout
	})
}

// Type into Ref
func (m *BrowserManager) Type(ref, text string) error {
	m.mu.Lock()
	loc, ok := m.refs[ref]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("ref %s not found", ref)
	}

	// Playwright Fill clears it first, then types.
	// If you want to simulate character-by-character typing without clearing, use Type()
	return loc.Fill(text, playwright.LocatorFillOptions{
		Timeout: playwright.Float(5000),
	})
}

// Scroll
func (m *BrowserManager) Scroll(direction string) error {
	if err := m.EnsureContext(); err != nil {
		return err
	}

	script := "window.scrollBy(0, window.innerHeight * 0.8)"
	if direction == "up" {
		script = "window.scrollBy(0, -window.innerHeight * 0.8)"
	} else if direction == "top" {
		script = "window.scrollTo(0, 0)"
	} else if direction == "bottom" {
		script = "window.scrollTo(0, document.body.scrollHeight)"
	}

	_, err := m.page.Evaluate(script)

	// Wait a moment for dynamic lazy-loaded content
	time.Sleep(500 * time.Millisecond)

	return err
}

// GetText
func (m *BrowserManager) GetText(ref string) (string, error) {
	m.mu.Lock()
	loc, ok := m.refs[ref]
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("ref %s not found", ref)
	}

	// Return InnerText to strip HTML tags
	return loc.InnerText()
}

// cleanUp gracefully tears down Playwright instances
func (m *BrowserManager) cleanUp() {
	if m.page != nil {
		m.page.Close()
	}
	if m.context != nil {
		m.context.Close()
	}
	if m.browser != nil {
		m.browser.Close()
	}
	if m.pw != nil {
		m.pw.Stop()
	}
	m.page = nil
	m.context = nil
	m.browser = nil
	m.pw = nil
}

// Close
func (m *BrowserManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanUp()
	m.refs = nil
}
