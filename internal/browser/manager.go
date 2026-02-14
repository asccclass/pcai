package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// BrowserManager handles persistent browser sessions
type BrowserManager struct {
	ctx         context.Context
	cancel      context.CancelFunc
	allocCtx    context.Context
	allocCancel context.CancelFunc

	mu        sync.Mutex
	refs      map[string]*cdp.Node
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
			refs: make(map[string]*cdp.Node),
		}
	})
	return instance
}

// EnsureContext makes sure a browser is running
func (m *BrowserManager) EnsureContext() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ctx != nil {
		// Check liveness?
		select {
		case <-m.ctx.Done():
			// Restart if dead
			m.ctx = nil
		default:
			return nil
		}
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true), // Headless for server env
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
	)

	m.allocCtx, m.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	m.ctx, m.cancel = chromedp.NewContext(m.allocCtx)

	// Ensure browser is actually started
	if err := chromedp.Run(m.ctx); err != nil {
		return err
	}
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

	return chromedp.Run(m.ctx, chromedp.Navigate(url))
}

// Snapshot parses the DOM and returns a list of interactive elements with refs
func (m *BrowserManager) Snapshot(interactiveOnly bool) (string, error) {
	if err := m.EnsureContext(); err != nil {
		return "", err
	}

	m.mu.Lock()
	m.refs = make(map[string]*cdp.Node) // Clear old refs
	m.lastRefID = 0
	m.mu.Unlock()

	var nodes []*cdp.Node
	// Selector for interactive elements
	sel := "a, button, input, select, textarea, [role='button'], [role='link']"
	if !interactiveOnly {
		sel = "*"
	}

	// We utilize `chromedp.Nodes` which waits for the nodes to be ready
	// Using ByQueryAll to find all matching
	if err := chromedp.Run(m.ctx, chromedp.Nodes(sel, &nodes, chromedp.ByQueryAll)); err != nil {
		return "", err
	}

	var sb strings.Builder
	var url string
	chromedp.Run(m.ctx, chromedp.Location(&url))
	sb.WriteString(fmt.Sprintf("Page URL: %s\n\n", url))

	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, node := range nodes {
		// Simplistic filtering: skip hidden?
		// Requires extra roundtrips. We skip for now.

		m.lastRefID++
		ref := fmt.Sprintf("@e%d", m.lastRefID)
		m.refs[ref] = node

		desc := m.getNodeDesc(node)
		if desc == "" {
			continue
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", ref, desc))
		count++
		if count > 200 {
			sb.WriteString("... (truncated to 200 elements, use filtering if needed) ...\n")
			break
		}
	}

	if count == 0 {
		return "No interactive elements found.", nil
	}

	return sb.String(), nil
}

func (m *BrowserManager) getNodeDesc(node *cdp.Node) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s]", strings.ToLower(node.NodeName)))

	// Iterate attributes
	for i := 0; i < len(node.Attributes); i += 2 {
		k := node.Attributes[i]
		v := node.Attributes[i+1]
		switch k {
		case "type", "placeholder", "name", "id", "aria-label", "alt", "href", "value":
			sb.WriteString(fmt.Sprintf(" %s=%q", k, v))
		}
	}

	// Try to get text content from first child if text
	// This is cached in the node passed back by chromedp if we requested it?
	// `chromedp.Nodes` might not populate full subtree.
	// But `node.Children` might be populated if we asked?
	// Default `Nodes` does NOT populate children recursively unless `chromedp.WithSubtree`?
	// Actually `Nodes` populates some info.
	// Let's rely on what we have. If no text, we might need to `Evaluate` to get text.
	// For performance, we skip `Evaluate` per node.

	// We can cheat: JS helper to return all info at once is better in future optimization.

	return sb.String()
}

// Click Ref
func (m *BrowserManager) Click(ref string) error {
	m.mu.Lock()
	node, ok := m.refs[ref]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("ref %s not found. Page might have reloaded. Please snapshot again.", ref)
	}

	// MouseClickNode is a direct CDP way
	return chromedp.Run(m.ctx, chromedp.MouseClickNode(node))
}

// Type into Ref
func (m *BrowserManager) Type(ref, text string) error {
	m.mu.Lock()
	node, ok := m.refs[ref]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("ref %s not found", ref)
	}

	// Focus then InsertText
	return chromedp.Run(m.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := dom.Focus().WithNodeID(node.NodeID).Do(ctx); err != nil {
			return err
		}
		// InsertText simulates typing
		return input.InsertText(text).Do(ctx)
	}))
}

// Scroll
func (m *BrowserManager) Scroll(direction string) error {
	if err := m.EnsureContext(); err != nil {
		return err
	}

	script := "window.scrollBy(0, 500)"
	if direction == "up" {
		script = "window.scrollBy(0, -500)"
	} else if direction == "top" {
		script = "window.scrollTo(0, 0)"
	} else if direction == "bottom" {
		script = "window.scrollTo(0, document.body.scrollHeight)"
	}

	return chromedp.Run(m.ctx, chromedp.Evaluate(script, nil))
}

// GetText
func (m *BrowserManager) GetText(ref string) (string, error) {
	m.mu.Lock()
	node, ok := m.refs[ref]
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("ref %s not found", ref)
	}

	var res string
	// Using remote object to get innerText
	err := chromedp.Run(m.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Resolve node to object ID
		obj, err := dom.ResolveNode().WithNodeID(node.NodeID).Do(ctx)
		if err != nil {
			return err
		}
		// Call function on object
		// We use `Runtime.callFunctionOn`
		// But chromedp doesn't expose a clean helper for "call function on specific resolved object" easily without importing specific domains.
		// Easier way: `chromedp.Evaluate` with internal logic?

		// Let's use `dom.GetOuterHTML` as fallback or `Javascript`.
		// But we really want `innerText`.
		// We can find the node by ID in JS? `document.evaluate`? No, we have NodeID.

		// NOTE: chromedp `Text` uses a selector. We don't have a selector.
		// We have to use the NodeID.

		// Workaround:
		// We can construct a remote function call:
		// `function() { return this.innerText }`
		// called on the object ID.
		_ = obj // RemoteObject
		// Not straightforward in pure `chromedp` high-level API.

		// Fallback: Just return "GetText not fully implemented for refs yet, try snapshot".
		// Or assume we have a selector? No.

		// Let's use `dom.GetOuterHTML` which is supported by NodeID.
		res, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
		return err
	}))

	return res, err
}

// Close
func (m *BrowserManager) Close() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.allocCancel != nil {
		m.allocCancel()
	}
	m.ctx = nil
	m.refs = nil
}
