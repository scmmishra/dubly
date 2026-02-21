package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/chatwoot/dubly/internal/db"
	"github.com/chatwoot/dubly/internal/models"
)

type seedLink struct {
	slug  string
	dest  string
	title string
	tags  string
	notes string
	// weight controls relative click volume (higher = more clicks)
	weight float64
}

var links = []seedLink{
	{"docs", "https://www.chatwoot.com/docs/product", "Product Documentation", "docs", "Main docs landing page", 5.0},
	{"setup", "https://www.chatwoot.com/docs/self-hosted/deployment/docker", "Docker Setup Guide", "docs,setup", "Self-hosted Docker install instructions", 4.0},
	{"api", "https://www.chatwoot.com/developers/api", "API Reference", "docs,api", "Developer API documentation", 4.5},
	{"sdk", "https://www.chatwoot.com/docs/product/channels/live-chat/sdk/setup", "SDK Setup", "docs,sdk", "Live chat widget SDK setup", 3.5},
	{"channels", "https://www.chatwoot.com/docs/product/channels", "Channels Overview", "docs,channels", "All supported channels", 3.0},
	{"whatsapp", "https://www.chatwoot.com/docs/product/channels/whatsapp/whatsapp-cloud", "WhatsApp Cloud Setup", "docs,channels,whatsapp", "WhatsApp Business Cloud API integration", 4.2},
	{"webhooks", "https://www.chatwoot.com/docs/product/features/webhooks", "Webhooks Guide", "docs,integrations", "Configuring webhooks for events", 2.8},
	{"mobile", "https://www.chatwoot.com/docs/product/channels/live-chat/sdk/mobile", "Mobile SDK", "docs,sdk,mobile", "Mobile app SDK integration", 2.5},
	{"k8s", "https://www.chatwoot.com/docs/self-hosted/deployment/kubernetes", "Kubernetes Deploy", "docs,setup,k8s", "Helm chart + Kubernetes setup", 3.2},
	{"agents", "https://www.chatwoot.com/docs/product/features/agents", "Agent Management", "docs,features", "Managing agents and teams", 2.0},
	{"auto", "https://www.chatwoot.com/docs/product/features/automations", "Automations", "docs,features", "Automation rules and workflows", 2.7},
	{"csat", "https://www.chatwoot.com/docs/product/features/csat", "CSAT Surveys", "docs,features", "Customer satisfaction surveys", 1.8},
	{"labels", "https://www.chatwoot.com/docs/product/features/labels", "Labels & Tags", "docs,features", "Organizing conversations with labels", 1.5},
	{"reports", "https://www.chatwoot.com/docs/product/features/reports", "Reports & Analytics", "docs,features", "Conversation and agent reports", 2.3},
	{"inboxes", "https://www.chatwoot.com/docs/product/channels/email/create-channel", "Email Inbox Setup", "docs,channels,email", "Setting up email channel", 2.6},
	{"contrib", "https://www.chatwoot.com/docs/contributing-guide", "Contributing Guide", "community", "How to contribute to Chatwoot", 1.2},
	{"fb", "https://www.chatwoot.com/docs/product/channels/facebook", "Facebook Channel", "docs,channels,facebook", "Facebook Messenger integration", 2.9},
	{"telegram", "https://www.chatwoot.com/docs/product/channels/telegram", "Telegram Channel", "docs,channels,telegram", "Telegram bot integration", 2.4},
	{"sso", "https://www.chatwoot.com/docs/self-hosted/configuration/features/sso-saml", "SSO / SAML Setup", "docs,setup,sso", "Single sign-on configuration", 1.9},
	{"migrate", "https://www.chatwoot.com/docs/self-hosted/monitoring/upgrade", "Upgrade Guide", "docs,setup", "Version upgrade instructions", 1.6},
}

var referrers = []struct {
	domain string
	weight float64
}{
	{"google.com", 30},
	{"", 20}, // direct traffic
	{"github.com", 15},
	{"twitter.com", 8},
	{"reddit.com", 7},
	{"dev.to", 5},
	{"news.ycombinator.com", 5},
	{"linkedin.com", 4},
	{"stackoverflow.com", 3},
	{"producthunt.com", 2},
	{"t.co", 1},
}

var countries = []struct {
	country string
	weight  float64
}{
	{"US", 25},
	{"IN", 20},
	{"DE", 8},
	{"GB", 7},
	{"BR", 6},
	{"FR", 5},
	{"CA", 4},
	{"AU", 3},
	{"JP", 3},
	{"NL", 2},
	{"SG", 2},
	{"ID", 2},
	{"ES", 2},
	{"IT", 1.5},
	{"PL", 1.5},
	{"SE", 1},
	{"KR", 1},
	{"NG", 1},
	{"MX", 1},
	{"TR", 1},
}

var browsers = []struct {
	name    string
	version string
	weight  float64
}{
	{"Chrome", "120", 45},
	{"Chrome", "119", 10},
	{"Firefox", "121", 15},
	{"Safari", "17", 12},
	{"Edge", "120", 8},
	{"Safari", "16", 5},
	{"Chrome", "118", 3},
	{"Firefox", "120", 2},
}

var oses = []struct {
	name   string
	weight float64
}{
	{"Windows", 35},
	{"macOS", 25},
	{"Linux", 15},
	{"Android", 15},
	{"iOS", 10},
}

var deviceTypes = []struct {
	name   string
	weight float64
}{
	{"desktop", 65},
	{"mobile", 30},
	{"tablet", 5},
}

func weightedPick[T any](items []struct {
	v      T
	weight float64
}, rng *rand.Rand) T {
	var total float64
	for _, item := range items {
		total += item.weight
	}
	r := rng.Float64() * total
	for _, item := range items {
		r -= item.weight
		if r <= 0 {
			return item.v
		}
	}
	return items[len(items)-1].v
}

func main() {
	dbPath := os.Getenv("DUBLY_DB_PATH")
	if dbPath == "" {
		dbPath = "./dubly.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	rng := rand.New(rand.NewSource(42)) // deterministic seed
	domain := "chwt.app"
	now := time.Now().UTC()
	sixMonthsAgo := now.AddDate(0, -6, 0)

	fmt.Println("Seeding links...")

	// Create all links with staggered creation dates
	createdLinks := make([]models.Link, 0, len(links))
	for i, sl := range links {
		// Spread creation dates over the first month
		daysOffset := i * 2 // every 2 days
		createdAt := sixMonthsAgo.Add(time.Duration(daysOffset) * 24 * time.Hour)

		link := models.Link{
			Slug:        sl.slug,
			Domain:      domain,
			Destination: sl.dest,
			Title:       sl.title,
			Tags:        sl.tags,
			Notes:       sl.notes,
		}

		if err := models.CreateLink(database, &link); err != nil {
			log.Fatalf("create link %q: %v", sl.slug, err)
		}

		// Backdate the created_at
		_, err := database.Exec(`UPDATE links SET created_at = ?, updated_at = ? WHERE id = ?`, createdAt, createdAt, link.ID)
		if err != nil {
			log.Fatalf("backdate link %q: %v", sl.slug, err)
		}

		link.CreatedAt = createdAt
		createdLinks = append(createdLinks, link)
		fmt.Printf("  [%2d] chwt.app/%s → %s\n", link.ID, sl.slug, sl.title)
	}

	fmt.Println("\nGenerating clicks...")

	// Build weighted picker helpers
	type refEntry struct {
		domain string
	}
	type countryEntry struct {
		country string
	}
	type browserEntry struct {
		name    string
		version string
	}
	type osEntry struct {
		name string
	}
	type deviceEntry struct {
		name string
	}

	pickReferrer := func() string {
		var total float64
		for _, r := range referrers {
			total += r.weight
		}
		v := rng.Float64() * total
		for _, r := range referrers {
			v -= r.weight
			if v <= 0 {
				return r.domain
			}
		}
		return referrers[0].domain
	}

	pickCountry := func() string {
		var total float64
		for _, c := range countries {
			total += c.weight
		}
		v := rng.Float64() * total
		for _, c := range countries {
			v -= c.weight
			if v <= 0 {
				return c.country
			}
		}
		return countries[0].country
	}

	pickBrowser := func() (string, string) {
		var total float64
		for _, b := range browsers {
			total += b.weight
		}
		v := rng.Float64() * total
		for _, b := range browsers {
			v -= b.weight
			if v <= 0 {
				return b.name, b.version
			}
		}
		return browsers[0].name, browsers[0].version
	}

	pickOS := func() string {
		var total float64
		for _, o := range oses {
			total += o.weight
		}
		v := rng.Float64() * total
		for _, o := range oses {
			v -= o.weight
			if v <= 0 {
				return o.name
			}
		}
		return oses[0].name
	}

	pickDevice := func() string {
		var total float64
		for _, d := range deviceTypes {
			total += d.weight
		}
		v := rng.Float64() * total
		for _, d := range deviceTypes {
			v -= d.weight
			if v <= 0 {
				return d.name
			}
		}
		return deviceTypes[0].name
	}

	totalClicks := 0

	for i, sl := range links {
		link := createdLinks[i]

		// Base clicks per day scaled by weight (roughly 10-50 clicks/day for top links)
		baseClicksPerDay := sl.weight * 8

		// Generate clicks from link creation to now
		var clicks []models.Click
		day := link.CreatedAt

		for day.Before(now) {
			// Add some daily variance (±40%)
			dayVariance := 0.6 + rng.Float64()*0.8
			// Add a growth trend — more recent days get slightly more clicks
			daysSinceCreation := day.Sub(link.CreatedAt).Hours() / 24
			totalDays := now.Sub(link.CreatedAt).Hours() / 24
			growthFactor := 0.7 + 0.6*(daysSinceCreation/totalDays)

			// Weekend dip
			weekdayFactor := 1.0
			if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
				weekdayFactor = 0.4
			}

			clicksThisDay := int(baseClicksPerDay * dayVariance * growthFactor * weekdayFactor)
			if clicksThisDay < 0 {
				clicksThisDay = 0
			}

			for j := 0; j < clicksThisDay; j++ {
				// Random time during the day, weighted toward business hours
				hour := rng.NormFloat64()*4 + 14 // center around 2pm UTC
				if hour < 0 {
					hour = 0
				}
				if hour >= 24 {
					hour = 23
				}
				minute := rng.Intn(60)
				second := rng.Intn(60)

				clickTime := time.Date(day.Year(), day.Month(), day.Day(),
					int(hour), minute, second, 0, time.UTC)

				if clickTime.After(now) {
					continue
				}

				refDomain := pickReferrer()
				refURL := ""
				if refDomain != "" {
					refURL = "https://" + refDomain + "/"
				}

				browser, browserVer := pickBrowser()

				click := models.Click{
					LinkID:         link.ID,
					ClickedAt:      clickTime,
					IP:             fmt.Sprintf("%d.%d.%d.%d", rng.Intn(224)+1, rng.Intn(256), rng.Intn(256), rng.Intn(256)),
					UserAgent:      fmt.Sprintf("Mozilla/5.0 (%s) %s/%s", pickOS(), browser, browserVer),
					Referer:        refURL,
					RefererDomain:  refDomain,
					Country:        pickCountry(),
					Browser:        browser,
					BrowserVersion: browserVer,
					OS:             pickOS(),
					DeviceType:     pickDevice(),
				}

				clicks = append(clicks, click)
			}

			// Flush in batches of 500
			if len(clicks) >= 500 {
				if err := models.BatchInsertClicks(database, clicks); err != nil {
					log.Fatalf("insert clicks for %s: %v", sl.slug, err)
				}
				totalClicks += len(clicks)
				clicks = clicks[:0]
			}

			day = day.Add(24 * time.Hour)
		}

		// Flush remaining
		if len(clicks) > 0 {
			if err := models.BatchInsertClicks(database, clicks); err != nil {
				log.Fatalf("insert clicks for %s: %v", sl.slug, err)
			}
			totalClicks += len(clicks)
		}

		fmt.Printf("  chwt.app/%-10s  clicks generated\n", sl.slug)
	}

	fmt.Printf("\nDone! Created %d links with %d total clicks.\n", len(links), totalClicks)
	fmt.Printf("Database: %s\n", dbPath)
}
