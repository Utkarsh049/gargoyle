package rules

import (
	"context"
	"strings"

	"gargoyle/internal/abuse"
)

// HeaderAnomalyRule checks for missing or suspicious User-Agents and automated tool signatures.
type HeaderAnomalyRule struct{}

// NewHeaderAnomalyRule creates a HeaderAnomalyRule.
func NewHeaderAnomalyRule() *HeaderAnomalyRule {
	return &HeaderAnomalyRule{}
}

func (r *HeaderAnomalyRule) Name() string {
	return "header_anomaly"
}

// automatedToolSignatures lists case-insensitive User-Agent substrings for
// automated scripting clients, crawlers, and scanning utilities.
var automatedToolSignatures = []string{
	"curl/",
	"python-requests",
	"python-urllib",
	"go-http-client",
	"wget/",
	"libwww-perl",
	"scrapy",
	"aiohttp",
	"httpx/",
	"postmanruntime",
	"sqlmap",
	"nikto",
	"nmap",
	"masscan",
	"zgrab",
}

// headlessSignatures lists signatures for headless automation frameworks.
var headlessSignatures = []string{
	"headlesschrome",
	"phantomjs",
	"puppeteer",
	"playwright",
	"selenium",
}

func (r *HeaderAnomalyRule) Evaluate(_ context.Context, req *abuse.RequestContext) (abuse.Decision, error) {
	if req == nil {
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	ua := strings.TrimSpace(req.UserAgent)

	// 1. Missing or blank User-Agent
	if ua == "" {
		return abuse.Decision{
			Action: abuse.ActionBlock,
			Score:  0.85,
			Rule:   r.Name(),
			Reason: "missing or empty User-Agent header",
		}, nil
	}

	lowerUA := strings.ToLower(ua)

	// 2. Automated tool / scraper signatures
	for _, sig := range automatedToolSignatures {
		if strings.Contains(lowerUA, sig) {
			return abuse.Decision{
				Action: abuse.ActionBlock,
				Score:  0.90,
				Rule:   r.Name(),
				Reason: "automated scraper or tool signature in User-Agent: " + sig,
			}, nil
		}
	}

	// 3. Headless browser automation signatures
	for _, sig := range headlessSignatures {
		if strings.Contains(lowerUA, sig) {
			return abuse.Decision{
				Action: abuse.ActionBlock,
				Score:  0.85,
				Rule:   r.Name(),
				Reason: "headless browser automation signature in User-Agent: " + sig,
			}, nil
		}
	}

	// 4. Header consistency check:
	// If a client claims to be a mainstream desktop browser (Chrome/Mozilla/Safari),
	// it should supply standard browser headers (Accept).
	if isBrowserClaim(lowerUA) {
		if req.Header == nil || req.Header.Get("Accept") == "" {
			return abuse.Decision{
				Action: abuse.ActionBlock,
				Score:  0.80,
				Rule:   r.Name(),
				Reason: "browser User-Agent claimed without standard Accept header",
			}, nil
		}
	}

	return abuse.Decision{
		Action: abuse.ActionAllow,
		Score:  0.0,
		Rule:   r.Name(),
		Reason: "",
	}, nil
}

func isBrowserClaim(lowerUA string) bool {
	return strings.Contains(lowerUA, "mozilla/5.0") &&
		(strings.Contains(lowerUA, "chrome/") || strings.Contains(lowerUA, "safari/") || strings.Contains(lowerUA, "firefox/"))
}
