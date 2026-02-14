---
name: browser_automation
description: Browser automation tools for AI agents. Use when the user needs to interact with websites, including navigating pages, filling forms, clicking buttons, extracting data, or automating any browser task. Triggers include requests to "open a website", "search for...", "click button", "fill form", "take screenshot" (not yet impl), or "scrape data".
allowed-tools: browser_open, browser_snapshot, browser_click, browser_type, browser_scroll, browser_get
---

# Browser Automation Skills

## Core Workflow

Every browser automation follows this pattern:

1.  **Navigate**: `browser_open`
2.  **Snapshot**: `browser_snapshot` (get element refs like `@e1`, `@e2`)
3.  **Interact**: Use refs to click, fill, select
4.  **Re-snapshot**: After navigation or DOM changes, get fresh refs

```json
// 1. Open URL
{ "url": "https://example.com/form" } // browser_open

// 2. Snapshot
{ "interactive_only": true } // browser_snapshot
// Output: @e1 [input type="email"], @e2 [input type="password"], @e3 [button] "Submit"

// 3. Interact
{ "ref": "@e1", "text": "user@example.com" } // browser_type
{ "ref": "@e2", "text": "password123" } // browser_type
{ "ref": "@e3" } // browser_click

// 4. Check result (Re-snapshot)
{ "interactive_only": true } // browser_snapshot
```

## Available Tools

### 1. Navigation
- **`browser_open`**
    - usage: `browser_open(url="https://google.com")`
    - description: Opens the browser and navigates to the URL.

### 2. Page Analysis (Snapshot)
- **`browser_snapshot`**
    - usage: `browser_snapshot(interactive_only=true)`
    - description: Analyzes the current page and returns a list of interactive elements with ID references (`@eN`).
    - **Important**: Refs (`@e1`, etc.) are temporary. Always re-snapshot after a page load or significant interaction.

### 3. Interaction
- **`browser_click`**
    - usage: `browser_click(ref="@e1")`
    - description: Clicks on an element identified by a ref.
- **`browser_type`**
    - usage: `browser_type(ref="@e1", text="hello")`
    - description: Types text into an element.
- **`browser_scroll`**
    - usage: `browser_scroll(direction="down")`
    - description: Scrolls the page. Directions: `up`, `down`, `top`, `bottom`.

### 4. Extraction
- **`browser_get`**
    - usage: `browser_get(ref="@e1", what="text")`
    - description: Gets the text content (or html) of an element.

## Best Practices

1.  **Always Snapshot First**: You cannot interact with elements without a ref.
2.  **Re-Snapshot Frequently**: If you click a button that loads a new page or opens a modal, your old refs (`@e1`) are invalid. Call `browser_snapshot` again.
3.  **Check Output**: `browser_snapshot` output tells you what elements are available. Read it carefully.

## Examples

### Google Search
```json
[
  { "tool": "browser_open", "args": { "url": "https://www.google.com" } },
  { "tool": "browser_snapshot", "args": {} },
  // Agent sees @e4 [input title="Search"]
  { "tool": "browser_type", "args": { "ref": "@e4", "text": "Golang tutorials" } },
  { "tool": "browser_click", "args": { "ref": "@e4" } } // Often typing + enter or clicking search btn
]
```
