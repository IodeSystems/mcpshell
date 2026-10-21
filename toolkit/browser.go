package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	. "github.com/iodesystems/mcpshell/runtime"
)

const browserTimeout = 10 * time.Second

// browserToolkit drives a headless Chrome via the DevTools Protocol. The
// browser is launched lazily on the first command and torn down by Close.
type browserToolkit struct {
	mu         sync.Mutex
	headless   bool
	allocCtx   context.Context
	browserCtx context.Context
	cancels    []context.CancelFunc
}

// InstallBrowser registers the Browser.* toolkit — chromedp-backed browser
// automation. A Chrome/Chromium binary must be on PATH; it launches on first
// use. The returned Closer shuts the browser down.
func InstallBrowser(sh *Shell, headless bool) interface{ Close() error } {
	bt := &browserToolkit{headless: headless}
	bt.register(sh)
	return bt
}

func (bt *browserToolkit) Close() error {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	for i := len(bt.cancels) - 1; i >= 0; i-- {
		bt.cancels[i]()
	}
	bt.cancels = nil
	bt.browserCtx = nil
	return nil
}

// ensure lazily launches the browser. Caller must hold bt.mu. The browser is
// started on the persistent context (not a per-command timeout child) so that
// canceling a command's deadline cannot tear the browser down.
func (bt *browserToolkit) ensure() context.Context {
	if bt.browserCtx != nil {
		return bt.browserCtx
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.NoSandbox, chromedp.Flag("disable-dev-shm-usage", true))
	if !bt.headless {
		opts = append(opts, chromedp.VisibleWindow)
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	if err := chromedp.Do(browserCtx); err != nil {
		cancelBrowser()
		cancelAlloc()
		panic(Runtime("Browser: failed to launch Chrome — is it installed? " + err.Error()))
	}
	bt.allocCtx = allocCtx
	bt.browserCtx = browserCtx
	bt.cancels = []context.CancelFunc{cancelBrowser, cancelAlloc}
	return browserCtx
}

// do executes value-less actions under a deadline. Caller must hold bt.mu.
func (bt *browserToolkit) do(timeout time.Duration, steps ...chromedp.Action[chromedp.Void]) error {
	ctx, cancel := context.WithTimeout(bt.ensure(), timeout)
	defer cancel()
	return chromedp.Do(ctx, steps...)
}

func (bt *browserToolkit) mustDo(steps ...chromedp.Action[chromedp.Void]) {
	if err := bt.do(browserTimeout, steps...); err != nil {
		panic(Runtime("Browser: " + err.Error()))
	}
}

// mustGet runs one action that returns a value. Caller must hold bt.mu.
// A function, not a method: Go methods cannot have type parameters.
func mustGet[T any](bt *browserToolkit, action chromedp.Action[T]) T {
	ctx, cancel := context.WithTimeout(bt.ensure(), browserTimeout)
	defer cancel()
	v, err := chromedp.Run(ctx, action)
	if err != nil {
		panic(Runtime("Browser: " + err.Error()))
	}
	return v
}

func (bt *browserToolkit) register(sh *Shell) {
	sh.RegisterGuide("Browser", browserGuide)

	reg := func(name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Namespace: "Browser", Name: name, Signature: sig, Description: desc, Examples: examples,
			Fn: func(args []Value) Value {
				bt.mu.Lock()
				defer bt.mu.Unlock()
				return fn(args)
			}})
	}

	reg("open", "url: string, opts?: {wait?: string}",
		"navigates to a URL and waits for the page load event",
		[]string{`Browser.open("https://example.com")`},
		func(args []Value) Value {
			url := requireStringArg("Browser.open", args, 0)
			// One action, so navigate + location + title share one deadline.
			return mustGet(bt, func(ctx context.Context, t *chromedp.Target) (*ObjectVal, error) {
				if _, err := chromedp.Navigate(url)(ctx, t); err != nil {
					return nil, err
				}
				loc, err := chromedp.Location()(ctx, t)
				if err != nil {
					return nil, err
				}
				title, err := chromedp.Title()(ctx, t)
				if err != nil {
					return nil, err
				}
				o := NewObject()
				o.Set("url", Str(loc))
				o.Set("title", Str(title))
				return o, nil
			})
		})

	reg("click", "selector: string", "clicks an element matching the CSS selector",
		[]string{`Browser.click("button.submit")`},
		func(args []Value) Value {
			bt.mustDo(chromedp.Click(chromedp.CSS(requireStringArg("Browser.click", args, 0))))
			return Null
		})

	reg("type", "selector: string, text: string", "sets the value of an input element",
		[]string{`Browser.type("input[name=q]", "search query")`},
		func(args []Value) Value {
			sel := requireStringArg("Browser.type", args, 0)
			text := requireStringArg("Browser.type", args, 1)
			bt.mustDo(chromedp.SetValue(chromedp.CSS(sel), text))
			return Null
		})

	reg("text", "selector?: string",
		"gets visible text; without a selector, the whole page body",
		[]string{`Browser.text("h1")`, `Browser.text()`},
		func(args []Value) Value {
			sel := optString(args, 0, "body")
			return Str(mustGet(bt, chromedp.Text(chromedp.CSS(sel))))
		})

	reg("html", "selector?: string",
		"gets the inner HTML of an element, or the full page HTML",
		[]string{`Browser.html(".content")`, `Browser.html()`},
		func(args []Value) Value {
			if sel := optString(args, 0, ""); sel != "" {
				return Str(mustGet(bt, chromedp.InnerHTML(chromedp.CSS(sel))))
			}
			return Str(mustGet(bt, chromedp.OuterHTML(chromedp.CSS("html"))))
		})

	reg("select", "selector: string",
		"queries the live DOM with a CSS selector. Returns [{text, html, tag, attrs}]",
		[]string{`Browser.select("a.nav-link")`, `Browser.select("table tr")`},
		func(args []Value) Value {
			sel := requireStringArg("Browser.select", args, 0)
			selJSON, _ := json.Marshal(sel)
			js := "Array.from(document.querySelectorAll(" + string(selJSON) + ")).map(el => ({" +
				"text: el.textContent || '', html: el.innerHTML || '', " +
				"tag: el.tagName.toLowerCase(), " +
				"attrs: Object.fromEntries(Array.from(el.attributes).map(a => [a.name, a.value]))}))"
			res := mustGet(bt, chromedp.Evaluate[[]struct {
				Text  string            `json:"text"`
				HTML  string            `json:"html"`
				Tag   string            `json:"tag"`
				Attrs map[string]string `json:"attrs"`
			}](js))
			out := make([]Value, len(res))
			for i, r := range res {
				attrs := NewObject()
				keys := make([]string, 0, len(r.Attrs))
				for k := range r.Attrs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					attrs.Set(k, Str(r.Attrs[k]))
				}
				o := NewObject()
				o.Set("text", Str(r.Text))
				o.Set("html", Str(r.HTML))
				o.Set("tag", Str(r.Tag))
				o.Set("attrs", attrs)
				out[i] = o
			}
			return &ArrayVal{Elements: out}
		})

	reg("wait", "selector: string, opts?: {timeout?: number, state?: string}",
		`waits for an element. state: "visible" (default), "hidden", "attached", "detached"`,
		[]string{`Browser.wait(".results")`, `Browser.wait(".spinner", {state: "hidden"})`},
		func(args []Value) Value {
			sel := requireStringArg("Browser.wait", args, 0)
			opts, _ := argOpt(args, 1).(*ObjectVal)
			timeout := browserTimeout
			if ms := optObjInt(opts, "timeout"); ms != nil {
				timeout = time.Duration(*ms) * time.Millisecond
			}
			state, _ := optObjStr(opts, "state")
			var action chromedp.Action[chromedp.Void]
			switch state {
			case "hidden":
				action = chromedp.WaitNotVisible(chromedp.CSS(sel))
			case "attached":
				action = chromedp.WaitReady(chromedp.CSS(sel))
			case "detached":
				action = chromedp.WaitNotPresent(chromedp.CSS(sel))
			default:
				action = chromedp.WaitVisible(chromedp.CSS(sel))
			}
			if err := bt.do(timeout, action); err != nil {
				panic(Runtime("Browser.wait: " + err.Error()))
			}
			return Null
		})

	reg("screenshot", "path: string, opts?: {fullPage?: boolean}",
		"saves a screenshot to a file. fullPage: true captures the whole scrollable page",
		[]string{`Browser.screenshot("page.png")`},
		func(args []Value) Value {
			path := requireStringArg("Browser.screenshot", args, 0)
			opts, _ := argOpt(args, 1).(*ObjectVal)
			var buf []byte
			if optObjBool(opts, "fullPage") {
				// Quality 100 selects PNG, matching CaptureScreenshot; anything lower is JPEG.
				buf = mustGet(bt, chromedp.FullScreenshot(100))
			} else {
				buf = mustGet(bt, chromedp.CaptureScreenshot())
			}
			if err := os.WriteFile(path, buf, 0o644); err != nil {
				panic(Runtime("Browser.screenshot: " + err.Error()))
			}
			return Str(path)
		})

	reg("eval", "js: string", "executes JavaScript in the page and returns the result",
		[]string{`Browser.eval("document.title")`, `Browser.eval("document.querySelectorAll('a').length")`},
		func(args []Value) Value {
			js := requireStringArg("Browser.eval", args, 0)
			ctx, cancel := context.WithTimeout(bt.ensure(), browserTimeout)
			defer cancel()
			// T = any can be nil, so null and undefined both decode to nil.
			v, err := chromedp.Run(ctx, chromedp.Evaluate[any](js))
			if err != nil {
				panic(Runtime("Browser.eval: " + err.Error()))
			}
			return jsResultToShell(v)
		})

	reg("url", "", "returns the current page URL", []string{`Browser.url()`},
		func(_ []Value) Value {
			return Str(mustGet(bt, chromedp.Location()))
		})

	reg("title", "", "returns the current page title", []string{`Browser.title()`},
		func(_ []Value) Value {
			return Str(mustGet(bt, chromedp.Title()))
		})
}

// jsResultToShell converts a decoded JSON value (from Browser.eval) to a value.
func jsResultToShell(v any) Value {
	switch x := v.(type) {
	case nil:
		return Null
	case bool:
		return Bln(x)
	case float64:
		return Num(x)
	case string:
		return Str(x)
	case []any:
		out := make([]Value, len(x))
		for i, e := range x {
			out[i] = jsResultToShell(e)
		}
		return &ArrayVal{Elements: out}
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		o := NewObject()
		for _, k := range keys {
			o.Set(k, jsResultToShell(x[k]))
		}
		return o
	default:
		return Str(fmt.Sprint(x))
	}
}

const browserGuide = `Browser — chromedp-backed browser automation (namespaced as Browser.*)

TYPICAL: Navigate and read
  Browser.open("https://example.com")   // navigate, wait for load
  Browser.title()                       // → "Example Domain"
  Browser.text("h1")                     // visible text of an element
  Browser.text()                         // all visible text on the page

TYPICAL: Interact with forms
  Browser.type("input[name=q]", "mcpshell")
  Browser.click("button[type=submit]")
  Browser.wait(".results")               // wait for an element to appear
  Browser.select(".results .item") |> map(r => r.text)

TYPICAL: Screenshots and JavaScript
  Browser.screenshot("page.png")
  Browser.eval("document.querySelectorAll('a').length")

NOTES:
  - The browser launches on first use; a Chrome/Chromium binary must be on PATH
  - Headless by default; default element timeout is 10s`
