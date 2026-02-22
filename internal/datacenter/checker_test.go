package datacenter

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ── Helpers ─────────────────────────────────────────────────────────

// testChecker creates a Checker with manually loaded ranges and IPs,
// no background goroutines.
func testChecker(t *testing.T, cidrs []string, blockedIPs []string) *Checker {
	t.Helper()
	c := &Checker{
		blockedIPs: make(map[string]bool),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go func() { <-c.stop; close(c.done) }()

	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatalf("bad test CIDR %q: %v", cidr, err)
		}
		c.ranges = append(c.ranges, ipNet)
	}
	for _, ip := range blockedIPs {
		c.blockedIPs[ip] = true
	}
	t.Cleanup(func() { c.Shutdown() })
	return c
}

// serveText starts an httptest server that returns the given body.
func serveText(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ── IsBlocked: CIDR ranges ─────────────────────────────────────────

func TestIsBlocked_MatchesCIDRRange(t *testing.T) {
	c := testChecker(t, []string{"10.0.0.0/8", "192.168.1.0/24"}, nil)

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"192.168.1.100", true},
		{"192.168.2.1", false},
		{"8.8.8.8", false},
	}
	for _, tt := range tests {
		if got := c.IsBlocked(tt.ip); got != tt.want {
			t.Errorf("IsBlocked(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

// ── IsBlocked: individual IPs ───────────────────────────────────────

func TestIsBlocked_MatchesBlockedIP(t *testing.T) {
	c := testChecker(t, nil, []string{"1.2.3.4", "5.6.7.8"})

	if !c.IsBlocked("1.2.3.4") {
		t.Error("expected 1.2.3.4 to be blocked")
	}
	if !c.IsBlocked("5.6.7.8") {
		t.Error("expected 5.6.7.8 to be blocked")
	}
	if c.IsBlocked("9.9.9.9") {
		t.Error("expected 9.9.9.9 to NOT be blocked")
	}
}

// ── IsBlocked: combined check ───────────────────────────────────────

func TestIsBlocked_CombinesCIDRAndIndividualIPs(t *testing.T) {
	c := testChecker(t, []string{"10.0.0.0/8"}, []string{"203.0.113.50"})

	if !c.IsBlocked("10.1.2.3") {
		t.Error("CIDR match should block 10.1.2.3")
	}
	if !c.IsBlocked("203.0.113.50") {
		t.Error("individual IP match should block 203.0.113.50")
	}
	if c.IsBlocked("8.8.8.8") {
		t.Error("8.8.8.8 should not be blocked")
	}
}

// ── IsBlocked: edge cases ───────────────────────────────────────────

func TestIsBlocked_InvalidIP_ReturnsFalse(t *testing.T) {
	c := testChecker(t, []string{"0.0.0.0/0"}, nil)

	if c.IsBlocked("not-an-ip") {
		t.Error("invalid IP should return false")
	}
	if c.IsBlocked("") {
		t.Error("empty string should return false")
	}
}

func TestIsBlocked_EmptyChecker_ReturnsFalse(t *testing.T) {
	c := testChecker(t, nil, nil)

	if c.IsBlocked("8.8.8.8") {
		t.Error("empty checker should never block")
	}
}

// ── IsBlocked: concurrency ──────────────────────────────────────────

func TestIsBlocked_ConcurrentReads(t *testing.T) {
	c := testChecker(t, []string{"10.0.0.0/8"}, []string{"1.2.3.4"})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.IsBlocked("10.0.0.1")
			c.IsBlocked("1.2.3.4")
			c.IsBlocked("8.8.8.8")
		}()
	}
	wg.Wait()
}

// ── parseIPRanges ───────────────────────────────────────────────────

func TestParseIPRanges_ValidCIDRs(t *testing.T) {
	input := "10.0.0.0/8\n172.16.0.0/12\n192.168.0.0/16\n"
	nets, err := parseIPRanges(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 3 {
		t.Fatalf("got %d ranges, want 3", len(nets))
	}
}

func TestParseIPRanges_SkipsCommentsAndBlanks(t *testing.T) {
	input := `# This is a comment
10.0.0.0/8

# Another comment
192.168.0.0/16
`
	nets, err := parseIPRanges(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("got %d ranges, want 2", len(nets))
	}
}

func TestParseIPRanges_SkipsInvalidLines(t *testing.T) {
	input := "10.0.0.0/8\nnot-a-cidr\n192.168.0.0/16\n"
	nets, err := parseIPRanges(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("got %d ranges, want 2 (invalid line skipped)", len(nets))
	}
}

// ── fetchIPList (Tor / Greensnow format) ────────────────────────────

func TestFetchIPList_ParsesPlainIPs(t *testing.T) {
	srv := serveText(t, "1.2.3.4\n5.6.7.8\n# comment\n\n9.10.11.12\n")
	origURL := torExitNodeURL
	defer func() {}()

	// Test the underlying parsing through a real HTTP server
	ips, err := fetchIPListFrom(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 3 {
		t.Fatalf("got %d IPs, want 3", len(ips))
	}
	_ = origURL

	want := map[string]bool{"1.2.3.4": true, "5.6.7.8": true, "9.10.11.12": true}
	for _, ip := range ips {
		if !want[ip] {
			t.Errorf("unexpected IP %q", ip)
		}
	}
}

func TestFetchIPList_SkipsInvalidLines(t *testing.T) {
	srv := serveText(t, "1.2.3.4\nnot-an-ip\n5.6.7.8\n")
	ips, err := fetchIPListFrom(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 {
		t.Fatalf("got %d IPs, want 2", len(ips))
	}
}

// ── fetchIpsumIPs format ────────────────────────────────────────────

func TestFetchIpsumIPs_ParsesTabSeparatedFormat(t *testing.T) {
	body := `# IPsum threat intelligence
# Generated on ...
1.2.3.4	3
5.6.7.8	7
`
	srv := serveText(t, body)
	ips, err := fetchIpsumIPsFrom(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 {
		t.Fatalf("got %d IPs, want 2", len(ips))
	}
	if ips[0] != "1.2.3.4" || ips[1] != "5.6.7.8" {
		t.Errorf("got %v, want [1.2.3.4, 5.6.7.8]", ips)
	}
}

// ── OCI JSON format ─────────────────────────────────────────────────

func TestFetchOCIRanges_ParsesJSON(t *testing.T) {
	body := `{
		"regions": [
			{
				"region": "us-ashburn-1",
				"cidrs": [
					{"cidr": "129.146.0.0/21"},
					{"cidr": "129.146.8.0/22"}
				]
			},
			{
				"region": "eu-frankfurt-1",
				"cidrs": [
					{"cidr": "138.1.0.0/20"}
				]
			}
		]
	}`
	srv := serveText(t, body)
	nets, err := fetchOCIRangesFrom(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 3 {
		t.Fatalf("got %d ranges, want 3", len(nets))
	}
}

// ── DigitalOcean CSV format ─────────────────────────────────────────

func TestFetchDORanges_ParsesCSV(t *testing.T) {
	body := `192.241.128.0/17,US,US-NY,New York
104.131.0.0/18,US,US-NJ,Clifton
`
	srv := serveText(t, body)
	nets, err := fetchDORangesFrom(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("got %d ranges, want 2", len(nets))
	}
}

// ── Vultr text format ───────────────────────────────────────────────

func TestFetchVultrRanges_ParsesPlainCIDRs(t *testing.T) {
	body := "45.32.0.0/15\n64.156.0.0/18\n# comment line\n"
	srv := serveText(t, body)
	nets, err := fetchVultrRangesFrom(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("got %d ranges, want 2", len(nets))
	}
}

// ── Hardcoded CIDRs parse correctly ─────────────────────────────────

func TestAkamaiCIDRs_AllValid(t *testing.T) {
	nets, err := parseCIDRList(akamaiCIDR)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != len(akamaiCIDR) {
		t.Errorf("parsed %d of %d Akamai CIDRs", len(nets), len(akamaiCIDR))
	}
}

func TestScalewayCIDRs_AllValid(t *testing.T) {
	nets, err := parseCIDRList(scalewayCIDR)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != len(scalewayCIDR) {
		t.Errorf("parsed %d of %d Scaleway CIDRs", len(nets), len(scalewayCIDR))
	}
}
