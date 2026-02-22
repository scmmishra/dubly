package analytics

import (
	"strings"

	"github.com/mssola/useragent"
)

// Substrings matched case-insensitively against the User-Agent.
var botSignatures = []string{
	// Generic patterns
	"bot",
	"spider",
	"crawl",

	// Link-preview / unfurler bots
	"facebookexternalhit",
	"facebot",
	"whatsapp",
	"slackbot",
	"telegrambot",
	"applebot",
	"twitterbot",
	"linkedinbot",
	"preview",

	// Google
	"google web preview",
	"google favicon",
	"google-ad",
	"google-site-verification",
	"googlesecurityscanner",
	"google_analytics_snippet_validator",
	"chrome-lighthouse",

	// Security / scanning
	"burpcollaborator.net/",
	"zgrab/",
	"netcraftsurveyagent/",
	"netcraft web server survey",

	// HTTP client libraries (not real browsers)
	"go-http-client/",
	"curl/",
	"wget/",
	"python-requests/",
	"python-urllib/",
	"pycurl/",
	"java/",
	"libwww-perl/",
	"okhttp/",
	"ruby",

	// Headless / renderers
	"headlesschrome/",
	"dumprendertree/",
	"phantomjs",
	"slimerjs",
	"wkhtmltoimage",
	"wkhtmltopdf",

	// Misc known bots & tools
	"admantx",
	"alexatoolbar/",
	"bingpreview/",
	"dataprovider.com",
	"faraday v",
	"gigablastopensource/",
	"owler/",
	"pageanalyzer/",
	"panscient.com",
	"ruxitrecorder/",
	"ruxitsynthetic/",
	"synapse",
	"tracemyfile/",
	"trendsmapresolver/",
	"ubermetrics-technologies.com",
	"wappalyzer",
	"whatweb/",
	"wininet",
	"wordpress.com",
	"wsr-agent/",
}

// IsBot returns true if the user-agent looks like a bot or link-preview fetcher.
func IsBot(rawUA string) bool {
	ua := useragent.New(rawUA)
	if ua.Bot() {
		return true
	}
	lower := strings.ToLower(rawUA)
	for _, sig := range botSignatures {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}
