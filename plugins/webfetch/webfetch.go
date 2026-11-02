// Package webfetch is the web_fetch tool plugin.
//
// It reads one page and returns it as text. Search tells the model what exists;
// this is how it reads one of them rather than guessing from a snippet.
//
// The stripping is deliberately simple — no parser dependency, no JavaScript — so
// what comes back is the text a reader would see, not a faithful copy of the page.
// Scripts and styles are removed first, since their contents are the usual reason
// stripped pages read like noise.
package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

func init() {
	plugin.Register("web_fetch", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools), nil
	})
}

const (
	// userAgent is a plain desktop one, because a fair number of sites answer
	// nothing at all to an unknown client.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36"

	// maxDownload caps what is read off the wire.
	maxDownload = 4 << 20

	// maxText caps what is handed back, so one page cannot fill a context window.
	maxText = 40_000

	// defaultText is the cap when an argument asks for less.
	defaultText = 12_000
)

// Plugin is the web_fetch tool.
type Plugin struct {
	registry *tools.Registry
	client   *http.Client
}

// New returns the plugin wired to a tool registry.
func New(reg *tools.Registry) *Plugin {
	return &Plugin{
		registry: reg,
		client:   &http.Client{Timeout: 25 * time.Second},
	}
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "web_fetch" }

// About says what this is, so the plugins screen can group it and say what it is
// for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Reads one web page and returns it as text, so a result can be read rather than guessed at.",
	}
}

// Setup registers the tool and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	return p.registry.Register(p)
}

// Description tells the model what this tool does.
func (p *Plugin) Description() string {
	return "Fetch a web page and return its text with the markup stripped. Use it after a search to read what a result actually says."
}

// Schema is the JSON Schema of the tool's arguments.
func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The page to read, http or https."},
			"chars": {"type": "integer", "description": "How much text to return. Defaults to 12000, capped at 40000."}
		},
		"required": ["url"]
	}`)
}

type args struct {
	URL   string `json:"url"`
	Chars int    `json:"chars"`
}

// Run fetches and returns the page as text.
func (p *Plugin) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("web_fetch: bad arguments: %w", err)
	}
	if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		return "", fmt.Errorf("web_fetch: a url starting http:// or https:// is required")
	}

	limit := a.Chars
	if limit <= 0 {
		limit = defaultText
	}
	if limit > maxText {
		limit = maxText
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web_fetch: the page answered %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload))
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}

	kind := resp.Header.Get("Content-Type")
	text := ToText(string(body), kind)
	if strings.TrimSpace(text) == "" {
		return "That page had no readable text. It may be a script-only page, or not a page at all.", nil
	}

	header := fmt.Sprintf("%s\n%s\n\n", a.URL, kind)
	if len(text) > limit {
		return header + text[:limit] + "\n…(truncated)", nil
	}
	return header + text, nil
}

var (
	// Go's regexp has no backreferences, so each of these is named rather than
	// matched by a captured tag.
	scriptStyle = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`),
	}
	comments   = regexp.MustCompile(`(?s)<!--.*?-->`)
	blocks     = regexp.MustCompile(`(?i)</(p|div|section|article|li|tr|h[1-6]|blockquote|pre|header|footer|nav)>`)
	breaks     = regexp.MustCompile(`(?i)<br\s*/?>`)
	anyTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	manyLines  = regexp.MustCompile(`\n{3,}`)
	manySpaces = regexp.MustCompile(`[ \t]{2,}`)
)

// ToText turns a page into readable text. Exported so a test can check it against
// saved markup rather than the network.
func ToText(page, contentType string) string {
	if strings.Contains(contentType, "text/plain") {
		return strings.TrimSpace(page)
	}

	s := page
	for _, re := range scriptStyle {
		s = re.ReplaceAllString(s, " ")
	}
	s = comments.ReplaceAllString(s, " ")
	s = breaks.ReplaceAllString(s, "\n")
	s = blocks.ReplaceAllString(s, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)

	// A non-breaking space is not a space to anything that compares strings, and
	// pages are full of them. Left alone, "Second para" is one word with a
	// character no one can see.
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = manySpaces.ReplaceAllString(s, " ")
	s = manyLines.ReplaceAllString(s, "\n\n")

	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

var _ tools.Tool = (*Plugin)(nil)
