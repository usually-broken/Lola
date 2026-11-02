// Package websearch is the web_search tool plugin.
//
// It answers a question by looking it up on DuckDuckGo's plain HTML endpoint and
// returning the top results — title, address and the snippet of each. It does not
// decide the answer itself: the model reads the snippets and answers, which is the
// only honest division of labour here.
//
// Two things worth knowing about the endpoint. A GET is refused with a 202 and a
// challenge page, so this posts a form the way a browser's search box does, with a
// user-agent and a referer. And the sponsored results are marked by pointing at
// duckduckgo.com/y.js, so they are dropped rather than reported as findings.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

func init() {
	plugin.Register("web_search", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools), nil
	})
}

const (
	// Endpoint is where the search form is posted.
	Endpoint = "https://html.duckduckgo.com/html/"

	// userAgent is a plain desktop one. The endpoint answers a challenge page to
	// anything that doesn't look like a browser.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36"

	// defaultLimit is how many results are returned when none is asked for.
	defaultLimit = 5

	// maxLimit keeps one call from flooding a context window.
	maxLimit = 10
)

// Result is one search result.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Plugin is the web_search tool.
type Plugin struct {
	registry *tools.Registry
	client   *http.Client
}

// New returns the plugin wired to a tool registry.
func New(reg *tools.Registry) *Plugin {
	return &Plugin{
		registry: reg,
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "web_search" }

// About says what this is, so the plugins screen can group it and say what it is
// for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Searches the web and returns the top results: title, address and a snippet of each.",
	}
}

// Setup registers the tool and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	return p.registry.Register(p)
}

// Description tells the model what this tool does.
func (p *Plugin) Description() string {
	return "Search the web. Returns the top results with a snippet of each, for reading — answer from them rather than quoting them blind."
}

// Schema is the JSON Schema of the tool's arguments.
func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "What to search for."},
			"limit": {"type": "integer", "description": "How many results, 1 to 10. Defaults to 5."}
		},
		"required": ["query"]
	}`)
}

type args struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// Run searches and returns what it found.
func (p *Plugin) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("web_search: bad arguments: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", fmt.Errorf("web_search: query is required")
	}

	results, err := p.search(ctx, a.Query, a.Limit)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "No results. Try different words, or fewer of them.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d results for %q:\n\n", len(results), a.Query)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// search posts the form and parses the results out of the page.
func (p *Plugin) search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	form := url.Values{"q": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("web_search: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://html.duckduckgo.com/")
	req.Header.Set("Accept", "text/html")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web_search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web_search: the search endpoint answered %s", resp.Status)
	}

	// A page larger than this is not a results page.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("web_search: %w", err)
	}

	return Parse(string(body), limit), nil
}

var (
	linkPattern    = regexp.MustCompile(`(?s)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	snippetPattern = regexp.MustCompile(`(?s)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	tagPattern     = regexp.MustCompile(`(?s)<[^>]+>`)
	spacePattern   = regexp.MustCompile(`\s+`)
)

// Parse pulls the results out of a results page. Exported so a test can run it
// against a saved page rather than the network.
func Parse(page string, limit int) []Result {
	links := linkPattern.FindAllStringSubmatch(page, -1)
	snips := snippetPattern.FindAllStringSubmatch(page, -1)

	out := make([]Result, 0, len(links))
	for i, m := range links {
		href := html.UnescapeString(m[1])

		// Sponsored results all route through y.js. They are not findings.
		if strings.Contains(href, "duckduckgo.com/y.js") {
			continue
		}

		r := Result{
			Title: clean(m[2]),
			URL:   href,
		}
		if i < len(snips) {
			r.Snippet = clean(snips[i][1])
		}
		if r.Title == "" && r.URL == "" {
			continue
		}

		out = append(out, r)
		if len(out) == limit {
			break
		}
	}
	return out
}

// clean turns a fragment of markup into a line of text.
func clean(s string) string {
	s = tagPattern.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(spacePattern.ReplaceAllString(s, " "))
}

var _ tools.Tool = (*Plugin)(nil)
