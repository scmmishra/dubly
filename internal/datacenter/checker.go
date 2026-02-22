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
	datacenterIPRangesURL = "https://raw.githubusercontent.com/jhassine/server-ip-addresses/master/data/datacenters.txt"
	ociCIDRURL            = "https://docs.cloud.oracle.com/en-us/iaas/tools/public_ip_ranges.json"
	doCIDRURL             = "https://www.digitalocean.com/geo/google.csv"
	vultrCIDRURL          = "https://geofeed.constant.com/?text"
	torExitNodeURL        = "https://check.torproject.org/torbulkexitlist"
	ipsumURL              = "https://raw.githubusercontent.com/stamparm/ipsum/master/ipsum.txt"
	greensnowURL          = "https://blocklist.greensnow.co/greensnow.txt"

	refreshInterval = 24 * time.Hour
	fetchTimeout    = 30 * time.Second
)

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

	if c.blockedIPs[ip] {
		return true
	}
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
	var newRanges []*net.IPNet
	newBlocked := make(map[string]bool)

	// CIDR range sources — all fetched concurrently
	cidrSources := []struct {
		name string
		fn   func() ([]*net.IPNet, error)
	}{
		{"main", func() ([]*net.IPNet, error) { return fetchCIDRsFrom(datacenterIPRangesURL) }},
		{"oci", func() ([]*net.IPNet, error) { return fetchOCIRangesFrom(ociCIDRURL) }},
		{"digitalocean", func() ([]*net.IPNet, error) { return fetchDORangesFrom(doCIDRURL) }},
		{"vultr", func() ([]*net.IPNet, error) { return fetchCIDRsFrom(vultrCIDRURL) }},
		{"akamai", func() ([]*net.IPNet, error) { return parseCIDRList(akamaiCIDR) }},
		{"scaleway", func() ([]*net.IPNet, error) { return parseCIDRList(scalewayCIDR) }},
	}

	for _, src := range cidrSources {
		wg.Add(1)
		go func(name string, fn func() ([]*net.IPNet, error)) {
			defer wg.Done()
			ranges, err := fn()
			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			}
			newRanges = append(newRanges, ranges...)
			mu.Unlock()
		}(src.name, src.fn)
	}

	// Individual IP blocklists — all fetched concurrently
	ipSources := []struct {
		name string
		fn   func() ([]string, error)
	}{
		{"tor", func() ([]string, error) { return fetchIPListFrom(torExitNodeURL) }},
		{"ipsum", func() ([]string, error) { return fetchIpsumIPsFrom(ipsumURL) }},
		{"greensnow", func() ([]string, error) { return fetchIPListFrom(greensnowURL) }},
	}

	for _, src := range ipSources {
		wg.Add(1)
		go func(name string, fn func() ([]string, error)) {
			defer wg.Done()
			ips, err := fn()
			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			}
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

// ── Fetchers ────────────────────────────────────────────────────────

// fetchCIDRsFrom downloads a plain text file of one CIDR per line.
func fetchCIDRsFrom(url string) ([]*net.IPNet, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseIPRanges(resp.Body)
}

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

// fetchIPListFrom downloads a plain text file of one IP per line.
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

// fetchIpsumIPsFrom parses the IPsum "ip<tab>score" format.
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
		fields := strings.Fields(line)
		if len(fields) > 0 && net.ParseIP(fields[0]) != nil {
			ips = append(ips, fields[0])
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
