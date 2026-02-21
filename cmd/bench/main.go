package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/scmmishra/dubly/internal/db"
	"github.com/scmmishra/dubly/internal/models"
)

const linkCount = 200

func main() {
	concurrency := flag.Int("c", 50, "number of concurrent workers")
	duration := flag.Duration("d", 10*time.Second, "benchmark duration")
	flag.Parse()

	fmt.Println("Dubly Redirect Benchmark")
	fmt.Println("========================")

	// 1. Build server binary
	fmt.Printf("Building server...     ")
	tmpDir, err := os.MkdirTemp("", "dubly-bench-*")
	if err != nil {
		fatal("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "dubly-server")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/server")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fatal("build server: %v", err)
	}
	fmt.Println("done")

	// 2. Seed database
	fmt.Printf("Seeding database...    ")
	dbPath := filepath.Join(tmpDir, "dubly.db")
	database, err := db.Open(dbPath)
	if err != nil {
		fatal("open db: %v", err)
	}

	slugs := make([]string, linkCount)
	for i := range linkCount {
		slug := fmt.Sprintf("bench-%03d", i+1)
		slugs[i] = slug
		link := &models.Link{
			Slug:        slug,
			Domain:      "127.0.0.1",
			Destination: fmt.Sprintf("https://example.com/%d", i+1),
		}
		if err := models.CreateLink(database, link); err != nil {
			database.Close()
			fatal("seed link %d: %v", i+1, err)
		}
	}
	database.Close()
	fmt.Printf("done (%d links)\n", linkCount)

	// 3. Start server
	fmt.Printf("Starting server...     ")
	port, err := freePort()
	if err != nil {
		fatal("find free port: %v", err)
	}

	srv := exec.Command(binPath)
	srvLog, err := os.Create(filepath.Join(tmpDir, "server.log"))
	if err != nil {
		fatal("create server log: %v", err)
	}
	defer srvLog.Close()
	srv.Stdout = srvLog
	srv.Stderr = srvLog
	srv.Env = append(os.Environ(),
		"DUBLY_PASSWORD=bench",
		"DUBLY_DOMAINS=127.0.0.1",
		fmt.Sprintf("DUBLY_PORT=%d", port),
		fmt.Sprintf("DUBLY_DB_PATH=%s", dbPath),
		"DUBLY_CACHE_SIZE=10000",
		"DUBLY_FLUSH_INTERVAL=1h",
		"DUBLY_BUFFER_SIZE=500000",
	)
	if err := srv.Start(); err != nil {
		fatal("start server: %v", err)
	}
	defer func() {
		srv.Process.Signal(syscall.SIGINT)
		srv.Wait()
	}()

	// 4. Wait for server ready
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitReady(baseURL+"/admin/login", 5*time.Second); err != nil {
		fatal("server not ready: %v", err)
	}
	fmt.Printf("ready (port %d)\n", port)

	// 5. Run benchmark
	fmt.Printf("Benchmarking...        %s, %d workers\n", *duration, *concurrency)

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: *concurrency,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	rng := rand.New(rand.NewSource(42))
	seeds := make([]int64, *concurrency)
	for i := range seeds {
		seeds[i] = rng.Int63()
	}

	var (
		mu        sync.Mutex
		latencies []time.Duration
		errors    int64
		reqCount  atomic.Int64
	)

	benchStart := time.Now()
	deadline := benchStart.Add(*duration)
	var wg sync.WaitGroup

	// Progress bar
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		totalSec := duration.Seconds()
		for {
			select {
			case <-done:
				printProgress(totalSec, totalSec, reqCount.Load())
				fmt.Println()
				return
			case <-ticker.C:
				elapsed := time.Since(benchStart).Seconds()
				if elapsed > totalSec {
					elapsed = totalSec
				}
				printProgress(elapsed, totalSec, reqCount.Load())
			}
		}
	}()

	for i := range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localRng := rand.New(rand.NewSource(seeds[i]))
			var localLats []time.Duration
			var localErrs int64

			for time.Now().Before(deadline) {
				slug := slugs[localRng.Intn(linkCount)]
				reqURL := baseURL + "/" + slug

				start := time.Now()
				resp, err := client.Get(reqURL)
				elapsed := time.Since(start)

				reqCount.Add(1)

				if err != nil {
					localErrs++
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				if resp.StatusCode != http.StatusFound {
					localErrs++
					continue
				}

				localLats = append(localLats, elapsed)
			}

			mu.Lock()
			latencies = append(latencies, localLats...)
			errors += localErrs
			mu.Unlock()
		}()
	}

	wg.Wait()
	close(done)
	time.Sleep(10 * time.Millisecond) // let progress goroutine print final line

	// 6. Report results
	total := int64(len(latencies)) + errors
	rps := float64(total) / duration.Seconds()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Println("")
	fmt.Println("Results")
	fmt.Println("-------")
	fmt.Printf("Requests:    %s\n", commaFmt(total))
	fmt.Printf("Errors:      %d\n", errors)
	fmt.Printf("RPS:         %.1f\n", rps)

	if len(latencies) > 0 {
		fmt.Printf("Latency p50: %s\n", fmtDur(percentile(latencies, 50)))
		fmt.Printf("Latency p95: %s\n", fmtDur(percentile(latencies, 95)))
		fmt.Printf("Latency p99: %s\n", fmtDur(percentile(latencies, 99)))
	}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

func waitReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func printProgress(elapsed, total float64, reqs int64) {
	const barWidth = 30
	frac := elapsed / total
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * barWidth)
	bar := make([]byte, barWidth)
	for i := range bar {
		if i < filled {
			bar[i] = '#'
		} else {
			bar[i] = '-'
		}
	}
	rps := float64(0)
	if elapsed > 0 {
		rps = float64(reqs) / elapsed
	}
	fmt.Printf("\r  [%s] %.0fs/%.0fs  %s reqs  %.0f rps",
		string(bar), elapsed, total, commaFmt(reqs), rps)
}

func fmtDur(d time.Duration) string {
	return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
}

func commaFmt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
