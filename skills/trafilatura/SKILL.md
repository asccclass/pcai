---
name: trafilatura
description: |
  網頁內容擷取工具。提供網頁抓取、文字萃取、Metadata 解析、爬蟲、Sitemap/Feed 解析等功能，輸出支援 TXT、Markdown、JSON、CSV、HTML、XML 六種格式。
  支援 Playwright headless browser 渲染 SPA/React/Vue 動態頁面，並自動偵測 84 種語言。
  既可作為 CLI 工具使用，也提供完整的 Go API 供程式整合。

  參數說明：
  -format: 輸出格式：txt、markdown、json、csv、html、xml
  -comments: 包含 comment
  -tables: 包含 table
  -images: 包含 image
  -links: 包含 link
  -no-fallback: disable fallback extraction
  -precision: favour precision over recall
  -recall: favour recall over precision
  -pretty: pretty-print JSON output
  -timeout: HTTP request timeout
  -user-agent: HTTP User-Agent
  -delay: delay between crawl requests
  -concurrency: concurrent download workers
  -max-urls: maximum URLs to crawl
  -crawl: crawl mode (BFS from seed)
  -sitemap: discover and use sitemap
  -same-host: restrict crawl to same hostname
  -headless: use Playwright headless Chromium to render SPA/JS pages
  -stdin: read HTML from stdin
  -url: source URL (for stdin/file mode metadata)
  -f: read HTML from a file
  -o: write output to a file
  -version: print version

command: |
  bin\trafilatura.exe {{url}} -format={{format}} -headless={{headless}} -crawl={{crawl}} -sitemap={{sitemap}} -max-urls={{max-urls}} -same-host={{same-host}} -comments={{comments}} -tables={{tables}} -images={{images}} -links={{links}} -precision={{precision}} -recall={{recall}} -pretty={{pretty}} -timeout={{timeout}} -user-agent={{user-agent}} -delay={{delay}} -concurrency={{concurrency}} -no-fallback={{no-fallback}} -stdin={{stdin}} -url={{url}} -f={{f}} -o={{o}}
---

# trafilatura Skill

## 0. 角色定義
你是一位高效能的**網頁抓取工具**。你的專長是從網頁中提取關鍵資訊、進行分類，並依據指定的格式輸出。你具備網頁除錯能力。

## 概述

**核心流程：**
```
URL → Fetch（HTTP 或 Headless Browser）→ Extract（內容萃取）→ Format（格式化輸出）
```

- **自動語言偵測**：使用 `pkg/langdetect`（whatlanggo）從文字內容偵測 84 種語言，無需依賴 `<html lang>` 屬性
- **SPA 支援**：`-headless` 模式使用 Playwright 驅動 Chromium，等待 JS 執行完畢後再萃取

## 快速使用

### CLI 方式

* trafilatura.exe 程式碼的位置在：bin 目錄下。

```powershell
# 擷取單一網頁（純文字）
.\trafilatura.exe https://example.com/article

# 輸出 JSON
.\trafilatura.exe -format json https://example.com/article

# 輸出 Markdown
.\trafilatura.exe -format markdown https://example.com/article

# 擷取 SPA/React/Vue 動態頁面（Playwright headless）
.\trafilatura.exe -headless https://react.dev/

# 從本地 HTML 檔案擷取
.\trafilatura.exe -f page.html -url https://example.com/page

# 從 stdin 讀取 HTML
cat page.html | .\trafilatura.exe -stdin -url https://example.com/page

# 爬取整個網站（BFS，最多 100 頁）
.\trafilatura.exe -crawl -max-urls 100 https://example.com

# 透過 Sitemap 批次擷取
.\trafilatura.exe -sitemap https://example.com

# 寫入輸出檔案
.\trafilatura.exe -format json -o result.json https://example.com/article
```

## 功能詳解

### SPA/JS 動態頁面（Headless 模式）

> 首次使用前需安裝 Chromium（執行一次）：
> ```bash
> go run github.com/playwright-community/playwright-go/cmd/playwright@latest install chromium
> ```

```bash
# CLI：加上 -headless 旗標
.\trafilatura.exe -headless -format json https://vue-app.example.com

# 爬蟲模式也支援 headless
.\trafilatura.exe -headless -crawl -max-urls 20 https://spa-site.example.com
```

### 語言自動偵測

`language` 欄位現在會從多個來源自動填入，優先順序如下：
1. Open Graph `og:locale`
2. Dublin Core `dc.language`
3. HTML `<html lang>` 屬性
4. HTTP-Equiv `content-language`
5. **（新）** 從萃取的文字內容自動偵測（`whatlanggo`，84 種語言）

```json
// 對未設定 lang 的中文頁面，現在會自動偵測
{ "language": "zh" }
```

### 情境：爬取整個網站並存成 Markdown

```bash
.\bin\trafilatura.exe -crawl -format markdown -max-urls 500 -same-host -o output.md https://blog.example.com
```

## 注意事項

- **Headless 安裝**：首次使用 `-headless` 前需執行一次 `playwright install chromium`。
- **反爬蟲**：使用 `-delay` 設定禮貌延遲，並可透過 `-user-agent` 自訂 UA；部分網站需搭配 Cookie 或特定標頭。
- **語言偵測限制**：文字少於 20 字元時偵測可能不準確，建議搭配完整文章內容使用。


## 負面約束 (Negative Constraints)
* **禁止抓取 API **：API 資料的抓取，請使用 `fetch_url` 工具。
