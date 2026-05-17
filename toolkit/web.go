package toolkit

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	. "github.com/iodesystems/mcpshell/runtime"
)

const (
	webMaxResponseBytes = 512_000
	webRequestTimeout   = 15 * time.Second
	webCacheTTL         = 60 * time.Second
)

type webCacheEntry struct {
	value Value
	at    time.Time
}

// InstallWeb registers the Web (HTTP + search) and Html (CSS querying) toolkits.
func InstallWeb(sh *Shell) *Shell {
	client := &http.Client{Timeout: webRequestTimeout}
	var mu sync.Mutex
	cache := map[string]webCacheEntry{}

	cacheGet := func(key string) (Value, bool) {
		mu.Lock()
		defer mu.Unlock()
		e, ok := cache[key]
		if !ok {
			return nil, false
		}
		if time.Since(e.at) > webCacheTTL {
			delete(cache, key)
			return nil, false
		}
		return e.value, true
	}
	cachePut := func(key string, v Value) {
		mu.Lock()
		cache[key] = webCacheEntry{value: v, at: time.Now()}
		mu.Unlock()
	}

	reg := func(ns, name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Namespace: ns, Name: name, Signature: sig, Description: desc, Examples: examples, Fn: fn})
	}

	installWebNamespace(sh, reg, client, cacheGet, cachePut, &mu, cache)
	installHtmlNamespace(sh, reg)
	return sh
}

func installWebNamespace(sh *Shell, reg func(ns, name, sig, desc string, examples []string, fn NativeFn),
	client *http.Client, cacheGet func(string) (Value, bool), cachePut func(string, Value),
	mu *sync.Mutex, cache map[string]webCacheEntry) {

	sh.RegisterGuide("Web", `Web — HTTP requests and web search (namespaced as Web.*)

TYPICAL: Fetch a URL
  Web.fetch("https://api.example.com/data")                  // GET → {status, headers, body}
  Web.fetch("https://api.example.com/data", {parse: "json"}) // auto-parse JSON body

TYPICAL: POST with body
  Web.fetch("https://api.example.com/items", {
    method: "POST", headers: {"Content-Type": "application/json"},
    body: toJson({name: "widget"})
  })

TYPICAL: Read a page as text / search
  Web.fetchText("https://example.com")
  Web.search("static site generators")   // → [{title, url, snippet}]

NOTES:
  - GET responses are cached for 60s (use {noCache: true} to bypass)
  - Response bodies over 512KB are truncated; requests time out after 15s`)

	reg("Web", "fetch",
		"url: string, opts?: {method?: string, headers?: object, body?: string, parse?: string, noCache?: boolean}",
		"HTTP request. Returns {status, headers, body}. parse: \"json\" auto-parses body. GET responses cached 60s",
		[]string{`Web.fetch("https://httpbin.org/get")`, `Web.fetch(url, {parse: "json"})`},
		func(args []Value) Value {
			url := requireStringArg("Web.fetch", args, 0)
			opts, _ := argOpt(args, 1).(*ObjectVal)
			method, _ := optObjStr(opts, "method")
			body, hasBody := optObjStr(opts, "body")
			parse, _ := optObjStr(opts, "parse")
			noCache := optObjBool(opts, "noCache")

			effectiveMethod := method
			if effectiveMethod == "" {
				if hasBody {
					effectiveMethod = "POST"
				} else {
					effectiveMethod = "GET"
				}
			}
			effectiveMethod = strings.ToUpper(effectiveMethod)

			cacheKey := ""
			if effectiveMethod == "GET" && !noCache {
				cacheKey = "GET\x00" + url
				if v, ok := cacheGet(cacheKey); ok {
					return v
				}
			}

			var bodyReader io.Reader
			if hasBody {
				bodyReader = strings.NewReader(body)
			}
			req, err := http.NewRequest(effectiveMethod, url, bodyReader)
			if err != nil {
				panic(Runtime("Web.fetch: invalid request: " + err.Error()))
			}
			if opts != nil {
				if h, ok := opts.Get("headers"); ok {
					if ho, ok := h.(*ObjectVal); ok {
						for _, k := range ho.Keys() {
							hv, _ := ho.Get(k)
							req.Header.Set(k, hv.Display())
						}
					}
				}
			}
			resp, err := client.Do(req)
			if err != nil {
				panic(Runtime("Web.fetch: request failed: " + err.Error()))
			}
			defer resp.Body.Close()

			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				panic(Runtime("Web.fetch: failed to read response: " + err.Error()))
			}
			bodyStr := string(raw)
			if len(bodyStr) > webMaxResponseBytes {
				bodyStr = bodyStr[:webMaxResponseBytes] +
					fmt.Sprintf("\n\n... truncated (%d bytes, limit %d)", len(raw), webMaxResponseBytes)
			}

			headers := NewObject()
			headerKeys := make([]string, 0, len(resp.Header))
			for k := range resp.Header {
				headerKeys = append(headerKeys, k)
			}
			sortStrings(headerKeys)
			for _, k := range headerKeys {
				headers.Set(strings.ToLower(k), Str(strings.Join(resp.Header[k], ", ")))
			}

			var bodyVal Value = Str(bodyStr)
			if parse == "json" {
				bodyVal = parseJSONSafe(bodyStr, resp.StatusCode)
			}

			result := NewObject()
			result.Set("status", Num(float64(resp.StatusCode)))
			result.Set("headers", headers)
			result.Set("body", bodyVal)

			if cacheKey != "" && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				cachePut(cacheKey, result)
			}
			return result
		})

	reg("Web", "fetchText", "url: string",
		"fetches URL and extracts visible text (strips HTML)",
		[]string{`Web.fetchText("https://example.com")`},
		func(args []Value) Value {
			url := requireStringArg("Web.fetchText", args, 0)
			cacheKey := "TEXT\x00" + url
			if v, ok := cacheGet(cacheKey); ok {
				return v
			}
			resp, err := client.Get(url)
			if err != nil {
				panic(Runtime("Web.fetchText: request failed: " + err.Error()))
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			htmlText := string(raw)
			if len(htmlText) > webMaxResponseBytes {
				htmlText = htmlText[:webMaxResponseBytes]
			}
			result := Str(htmlVisibleText(htmlText))
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				cachePut(cacheKey, result)
			}
			return result
		})

	reg("Web", "clearCache", "",
		"clears the HTTP response cache. Returns number of entries cleared",
		[]string{`Web.clearCache()`},
		func(_ []Value) Value {
			mu.Lock()
			n := len(cache)
			for k := range cache {
				delete(cache, k)
			}
			mu.Unlock()
			return Num(float64(n))
		})

	reg("Web", "search", "query: string, opts?: {limit?: number}",
		"web search via DuckDuckGo. Returns [{title, url, snippet}]",
		[]string{`Web.search("go generics", {limit: 5})`},
		func(args []Value) Value {
			query := requireStringArg("Web.search", args, 0)
			opts, _ := argOpt(args, 1).(*ObjectVal)
			limit := 10
			if l := optObjInt(opts, "limit"); l != nil {
				limit = *l
			}
			return duckDuckGoSearch(client, query, limit)
		})
}

func installHtmlNamespace(sh *Shell, reg func(ns, name, sig, desc string, examples []string, fn NativeFn)) {
	sh.RegisterGuide("Html", `Html — CSS selector querying and HTML parsing (namespaced as Html.*)

TYPICAL: Query elements
  Html.select("<div><p>hello</p></div>", "p")  // → [{text, html, tag, attrs}]
  Web.fetch(url).body |> Html.select("h1") |> map(h => h.text)

TYPICAL: Links / text / tables
  Html.links(page)               // → [{text, href}]
  Html.text("<p>Hi <b>there</b></p>")
  Html.table(page, "table.data") // → [{col: val, ...}] using <th> as keys

CSS SELECTORS: tag, .class, #id, [attr], tag > child, tag child, selector1, selector2`)

	reg("Html", "select", "html: string, selector: string",
		"queries HTML with CSS selector. Returns [{text, html, tag, attrs}]",
		[]string{`Html.select("<p>hello</p>", "p")`},
		func(args []Value) Value {
			doc := parseHTML(requireStringArg("Html.select", args, 0))
			selector := requireStringArg("Html.select", args, 1)
			var out []Value
			doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
				out = append(out, elementToValue(s))
			})
			return &ArrayVal{Elements: out}
		})

	reg("Html", "links", "html: string, selector?: string",
		"extracts links. Returns [{text, href}]. Optional selector filters",
		[]string{`Html.links(page)`, `Html.links(page, "nav a")`},
		func(args []Value) Value {
			doc := parseHTML(requireStringArg("Html.links", args, 0))
			selector := optString(args, 1, "")
			var anchors *goquery.Selection
			if selector != "" {
				sel := doc.Find(selector)
				anchors = sel.Find("a[href]")
				if anchors.Length() == 0 {
					anchors = sel.FilterFunction(func(_ int, s *goquery.Selection) bool {
						_, has := s.Attr("href")
						return goquery.NodeName(s) == "a" && has
					})
				}
			} else {
				anchors = doc.Find("a[href]")
			}
			var out []Value
			anchors.Each(func(_ int, s *goquery.Selection) {
				href, _ := s.Attr("href")
				o := NewObject()
				o.Set("text", Str(collapseWS(s.Text())))
				o.Set("href", Str(href))
				out = append(out, o)
			})
			return &ArrayVal{Elements: out}
		})

	reg("Html", "text", "html: string",
		"extracts visible text from HTML, stripping all tags",
		[]string{`Html.text("<p>Hello <b>world</b></p>")`},
		func(args []Value) Value {
			return Str(htmlVisibleText(requireStringArg("Html.text", args, 0)))
		})

	reg("Html", "table", "html: string, selector?: string",
		"extracts HTML table as array of objects. Uses <th> for keys, falls back to col0, col1",
		[]string{`Html.table(page, "table.data")`},
		func(args []Value) Value {
			doc := parseHTML(requireStringArg("Html.table", args, 0))
			selector := optString(args, 1, "table")
			table := doc.Find(selector).First()
			if table.Length() == 0 {
				panic(Runtime("Html.table: no table found matching '" + selector + "'"))
			}
			var headers []string
			table.Find("th").Each(func(_ int, s *goquery.Selection) {
				headers = append(headers, strings.TrimSpace(s.Text()))
			})
			var rows []Value
			table.Find("tr").Each(func(_ int, row *goquery.Selection) {
				cells := row.Find("td")
				if cells.Length() == 0 {
					return
				}
				o := NewObject()
				cells.Each(func(i int, cell *goquery.Selection) {
					key := fmt.Sprintf("col%d", i)
					if i < len(headers) {
						key = headers[i]
					}
					o.Set(key, Str(collapseWS(cell.Text())))
				})
				rows = append(rows, o)
			})
			return &ArrayVal{Elements: rows}
		})
}

// --- HTML helpers ------------------------------------------------------------

func parseHTML(htmlStr string) *goquery.Document {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		panic(Runtime("Html: parse error: " + err.Error()))
	}
	return doc
}

func elementToValue(s *goquery.Selection) *ObjectVal {
	attrs := NewObject()
	if len(s.Nodes) > 0 {
		for _, a := range s.Nodes[0].Attr {
			attrs.Set(a.Key, Str(a.Val))
		}
	}
	inner, _ := s.Html()
	o := NewObject()
	o.Set("text", Str(collapseWS(s.Text())))
	o.Set("html", Str(inner))
	o.Set("tag", Str(goquery.NodeName(s)))
	o.Set("attrs", attrs)
	return o
}

func htmlVisibleText(htmlStr string) string {
	doc := parseHTML(htmlStr)
	doc.Find("script, style, noscript").Remove()
	return collapseWS(doc.Text())
}

// collapseWS trims and collapses whitespace runs to single spaces.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// parseJSONSafe parses body as JSON, raising a Web.fetch-flavored error on failure.
func parseJSONSafe(body string, status int) (result Value) {
	defer func() {
		if r := recover(); r != nil {
			preview := body
			if len(preview) > 200 {
				preview = preview[:200]
			}
			panic(Runtime(fmt.Sprintf(
				"Web.fetch: failed to parse response as JSON\n\n  Status: %d\n  Body preview: %s\n\n"+
					"  Hint: remove {parse: \"json\"} to get raw body as string", status, preview)))
		}
	}()
	pos := 0
	return parseJSONValue(strings.TrimSpace(body), &pos)
}

// --- DuckDuckGo search -------------------------------------------------------

func duckDuckGoSearch(client *http.Client, query string, limit int) Value {
	form := url.Values{}
	form.Set("q", query)
	form.Set("b", "")
	req, err := http.NewRequest("POST", "https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
	if err != nil {
		panic(Runtime("Web.search: " + err.Error()))
	}
	req.Header.Set("User-Agent", "mcpshell/0.1")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		panic(Runtime("Web.search: request failed: " + err.Error()))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
	if err != nil {
		panic(Runtime("Web.search: failed to parse results"))
	}
	links := doc.Find("a.result__a")
	snippets := doc.Find("a.result__snippet")
	var results []Value
	links.Each(func(i int, s *goquery.Selection) {
		if len(results) >= limit {
			return
		}
		title := strings.TrimSpace(s.Text())
		href, _ := s.Attr("href")
		resolved := decodeDuckDuckGoURL(href)
		snippet := ""
		if i < snippets.Length() {
			snippet = strings.TrimSpace(snippets.Eq(i).Text())
		}
		if title != "" && resolved != "" {
			o := NewObject()
			o.Set("title", Str(title))
			o.Set("url", Str(resolved))
			o.Set("snippet", Str(snippet))
			results = append(results, o)
		}
	})
	return &ArrayVal{Elements: results}
}

func decodeDuckDuckGoURL(raw string) string {
	if !strings.Contains(raw, "duckduckgo.com/l/") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if uddg := u.Query().Get("uddg"); uddg != "" {
		return uddg
	}
	return raw
}
