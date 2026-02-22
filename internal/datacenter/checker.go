package datacenter

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// Datacenter CIDR sources
	datacenterIPRangesURL = "https://raw.githubusercontent.com/jhassine/server-ip-addresses/master/data/datacenters.txt"
	ociCIDRURL            = "https://docs.cloud.oracle.com/en-us/iaas/tools/public_ip_ranges.json"
	doCIDRURL             = "https://www.digitalocean.com/geo/google.csv"
	vultrCIDRURL          = "https://geofeed.constant.com/?text"

	// Threat / anonymizer IP sources
	torExitNodeURL = "https://check.torproject.org/torbulkexitlist"
	ipsumURL       = "https://raw.githubusercontent.com/stamparm/ipsum/master/ipsum.txt"
	greensnowURL   = "https://blocklist.greensnow.co/greensnow.txt"

	refreshInterval = 24 * time.Hour
	fetchTimeout    = 30 * time.Second
)

// Hardcoded CIDR ranges for providers without downloadable feeds.
var (
	akamaiCIDR = []string{
		"23.32.0.0/11", "23.192.0.0/11", "2.16.0.0/13", "104.64.0.0/10",
		"184.24.0.0/13", "23.0.0.0/12", "95.100.0.0/15", "92.122.0.0/15",
		"184.50.0.0/15", "88.221.0.0/16", "23.64.0.0/14", "72.246.0.0/15",
		"96.16.0.0/15", "96.6.0.0/15", "69.192.0.0/16", "23.72.0.0/13",
		"173.222.0.0/15", "118.214.0.0/16", "184.84.0.0/14",
	}
	scalewayCIDR = []string{
		"62.210.0.0/16", "195.154.0.0/16", "212.129.0.0/18", "62.4.0.0/19",
		"212.83.128.0/19", "212.83.160.0/19", "212.47.224.0/19", "163.172.0.0/16",
		"51.15.0.0/16", "151.115.0.0/16", "51.158.0.0/15",
	}
)

// Checker maintains in-memory datacenter CIDR ranges and threat IP blocklists.
// All lookups are thread-safe. Lists are refreshed periodically in the background.
type Checker struct {
	mu         sync.RWMutex
	ranges     []*net.IPNet
	blockedIPs map[string]bool
	stop       chan struct{}
	done       chan struct{}
}

// NewChecker starts a background goroutine that fetches all IP lists
// immediately and refreshes them every 24 hours.
func NewChecker() *Checker {
	c := &Checker{
		blockedIPs: make(map[string]bool),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go c.run()
	return c
}

// IsBlocked returns true if ip belongs to a known datacenter range,
// is a Tor exit node, or appears on a threat blocklist.
func (c *Checker) IsBlocked(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	// O(1) check against individual blocked IPs
	if c.blockedIPs[ip] {
		return true
	}

	// Linear scan of CIDR ranges
	for _, n := range c.ranges {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// Shutdown stops the background refresh and waits for it to finish.
func (c *Checker) Shutdown() {
	close(c.stop)
	<-c.done
}

func (c *Checker) run() {
	defer close(c.done)

	c.refresh()

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.refresh()
		case <-c.stop:
			return
		}
	}
}

func (c *Checker) refresh() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string

	// Fetch CIDR ranges
	var newRanges []*net.IPNet
	wg.Add(1)
	go func() {
		defer wg.Done()
		ranges, err := fetchAllRanges()
		if err != nil {
			mu.Lock()
			errs = append(errs, err.Error())
			mu.Unlock()
		}
		mu.Lock()
		newRanges = ranges
		mu.Unlock()
	}()

	// Fetch individual IP blocklists
	newBlocked := make(map[string]bool)
	ipSources := []struct {
		name string
		fn   func() ([]string, error)
	}{
		{"tor", fetchTorExitNodes},
		{"ipsum", fetchIpsumIPs},
		{"greensnow", fetchGreensnowIPs},
	}

	for _, src := range ipSources {
		wg.Add(1)
		go func(name string, fn func() ([]string, error)) {
			defer wg.Done()
			ips, err := fn()
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			for _, ip := range ips {
				newBlocked[ip] = true
			}
			mu.Unlock()
		}(src.name, src.fn)
	}

	wg.Wait()

	if len(errs) > 0 {
		log.Printf("ipcheck: partial refresh: %s", strings.Join(errs, "; "))
	}

	c.mu.Lock()
	if len(newRanges) > 0 {
		c.ranges = newRanges
	}
	if len(newBlocked) > 0 {
		c.blockedIPs = newBlocked
	}
	c.mu.Unlock()

	log.Printf("ipcheck: loaded %d CIDR ranges, %d blocked IPs", len(newRanges), len(newBlocked))
}

// ── CIDR range fetchers ─────────────────────────────────────────────

func fetchAllRanges() ([]*net.IPNet, error) {
	type result struct {
		ranges []*net.IPNet
		err    error
	}

	sources := []struct {
		name string
		fn   func() ([]*net.IPNet, error)
	}{
		{"main", fetchMainRanges},
		{"oci", fetchOCIRanges},
		{"digitalocean", fetchDORanges},
		{"vultr", fetchVultrRanges},
		{"akamai", func() ([]*net.IPNet, error) { return parseCIDRList(akamaiCIDR) }},
		{"scaleway", func() ([]*net.IPNet, error) { return parseCIDRList(scalewayCIDR) }},
	}

	results := make([]result, len(sources))
	var wg sync.WaitGroup

	for i, src := range sources {
		wg.Add(1)
		go func(idx int, name string, fn func() ([]*net.IPNet, error)) {
			defer wg.Done()
			r, err := fn()
			if err != nil {
				results[idx] = result{err: fmt.Errorf("%s: %w", name, err)}
				return
			}
			results[idx] = result{ranges: r}
		}(i, src.name, src.fn)
	}

	wg.Wait()

	var all []*net.IPNet
	var errs []string
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err.Error())
		}
		all = append(all, r.ranges...)
	}

	if len(errs) > 0 {
		return all, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return all, nil
}

func fetchMainRanges() ([]*net.IPNet, error) {
	resp, err := httpGet(datacenterIPRangesURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseIPRanges(resp.Body)
}

func fetchOCIRanges() ([]*net.IPNet, error)   { return fetchOCIRangesFrom(ociCIDRURL) }
func fetchDORanges() ([]*net.IPNet, error)    { return fetchDORangesFrom(doCIDRURL) }
func fetchVultrRanges() ([]*net.IPNet, error) { return fetchVultrRangesFrom(vultrCIDRURL) }

func fetchOCIRangesFrom(url string) ([]*net.IPNet, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Regions []struct {
			Cidrs []struct {
				Cidr string `json:"cidr"`
			} `json:"cidrs"`
		} `json:"regions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var cidrs []string
	for _, region := range data.Regions {
		for _, c := range region.Cidrs {
			cidrs = append(cidrs, c.Cidr)
		}
	}
	return parseCIDRList(cidrs)
}

func fetchDORangesFrom(url string) ([]*net.IPNet, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	var cidrs []string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) > 0 {
			cidrs = append(cidrs, record[0])
		}
	}
	return parseCIDRList(cidrs)
}

func fetchVultrRangesFrom(url string) ([]*net.IPNet, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseIPRanges(resp.Body)
}

// ── Individual IP fetchers ──────────────────────────────────────────

func fetchTorExitNodes() ([]string, error) { return fetchIPListFrom(torExitNodeURL) }
func fetchGreensnowIPs() ([]string, error) { return fetchIPListFrom(greensnowURL) }
func fetchIpsumIPs() ([]string, error)     { return fetchIpsumIPsFrom(ipsumURL) }

func fetchIpsumIPsFrom(url string) ([]string, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ips []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// IPsum format: "ip<tab>score"
		fields := strings.Fields(line)
		if len(fields) > 0 && net.ParseIP(fields[0]) != nil {
			ips = append(ips, fields[0])
		}
	}
	return ips, scanner.Err()
}

// fetchIPListFrom downloads a plain text list of one IP per line.
func fetchIPListFrom(url string) ([]string, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ips []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if net.ParseIP(line) != nil {
			ips = append(ips, line)
		}
	}
	return ips, scanner.Err()
}

// ── Parsing helpers ─────────────────────────────────────────────────

func httpGet(url string) (*http.Response, error) {
	client := &http.Client{Timeout: fetchTimeout}
	return client.Get(url)
}

func parseIPRanges(r io.Reader) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			continue
		}
		nets = append(nets, ipNet)
	}
	return nets, scanner.Err()
}

func parseCIDRList(cidrs []string) ([]*net.IPNet, error) {
	return parseIPRanges(strings.NewReader(strings.Join(cidrs, "\n")))
}
