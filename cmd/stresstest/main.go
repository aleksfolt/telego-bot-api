package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	ServerURL   string
	PprofURL    string
	Token       string
	Concurrency int
	Duration    time.Duration
	RateLimit   int
	Scenario    string
}

type LatencyStats struct {
	TotalReqs   int64
	SuccessReqs int64
	ErrorReqs   int64
	StatusCodes map[int]int64
	Latencies   []float64 // in ms
	Duration    time.Duration
	Min         float64
	Mean        float64
	Median      float64
	P90         float64
	P95         float64
	P99         float64
	Max         float64
	RPS         float64
}

type PprofMetrics struct {
	Goroutines int
	HeapAlloc  uint64
	HeapInuse  uint64
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.ServerURL, "server", "http://127.0.0.1:8082", "telego-bot-api server address")
	flag.StringVar(&cfg.PprofURL, "pprof", "http://127.0.0.1:6060", "pprof diagnostic server address (empty to disable)")
	flag.StringVar(&cfg.Token, "token", "7026718333:AAHIlgGrBLc4yWSv6JfYDArpRchYuXidNNM", "Bot token to stress test")
	flag.IntVar(&cfg.Concurrency, "concurrency", 100, "Number of concurrent worker goroutines")
	flag.DurationVar(&cfg.Duration, "duration", 20*time.Second, "Duration of the stress test")
	flag.IntVar(&cfg.RateLimit, "rate", 0, "Max requests per second (0 = unlimited)")
	flag.StringVar(&cfg.Scenario, "scenario", "mixed", "Scenario: throughput, mixed, burst, leak")
	flag.Parse()

	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")
	cfg.PprofURL = strings.TrimRight(cfg.PprofURL, "/")

	fmt.Println("================================================================================")
	fmt.Println("                TELEGO-BOT-API HIGH-LOAD STRESS TESTING SUITE                   ")
	fmt.Println("================================================================================")
	fmt.Printf(" Target Server  : %s\n", cfg.ServerURL)
	fmt.Printf(" Bot Token      : %s...\n", cfg.Token[:min(10, len(cfg.Token))])
	fmt.Printf(" Concurrency    : %d workers\n", cfg.Concurrency)
	fmt.Printf(" Duration       : %s\n", cfg.Duration)
	fmt.Printf(" Rate Limit     : %d RPS (0 = unlimited)\n", cfg.RateLimit)
	fmt.Printf(" Scenario       : %s\n", cfg.Scenario)
	fmt.Println("--------------------------------------------------------------------------------")

	// 1. Verify health endpoint first
	if err := checkHealth(cfg.ServerURL); err != nil {
		fmt.Printf("❌ Health check failed at %s/health: %v\n", cfg.ServerURL, err)
		os.Exit(1)
	}
	fmt.Println("✅ Target server is healthy and responding.")

	// 2. Capture baseline pprof metrics
	var baselinePprof PprofMetrics
	if cfg.PprofURL != "" {
		baselinePprof = fetchPprof(cfg.PprofURL)
		fmt.Printf("📊 Baseline Runtime: %d goroutines, %.2f MB heap inuse\n",
			baselinePprof.Goroutines, float64(baselinePprof.HeapInuse)/(1024*1024))
	}

	if cfg.Scenario == "webhook" {
		runWebhookScenario(cfg, baselinePprof)
		return
	}

	// 3. Run stress test
	stats := runStressTest(cfg)

	// 4. Cool-down & leak inspection
	var finalPprof PprofMetrics
	if cfg.PprofURL != "" {
		fmt.Print("\n⏳ Waiting 3s for GC and cool-down...\n")
		time.Sleep(3 * time.Second)
		finalPprof = fetchPprof(cfg.PprofURL)
	}

	// 5. Print comprehensive results
	printResults(stats, baselinePprof, finalPprof, cfg)
}

func checkHealth(serverURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func fetchPprof(pprofURL string) PprofMetrics {
	m := PprofMetrics{}
	client := &http.Client{Timeout: 3 * time.Second}

	// Get goroutines count
	resp, err := client.Get(pprofURL + "/debug/pprof/goroutine?debug=1")
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		lines := strings.Split(string(body), "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "goroutine profile: total ") {
			var total int
			if _, e := fmt.Sscanf(lines[0], "goroutine profile: total %d", &total); e == nil {
				m.Goroutines = total
			}
		}
	}

	// Read runtime memory
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)
	m.HeapAlloc = rtm.HeapAlloc
	m.HeapInuse = rtm.HeapInuse
	return m
}

func runStressTest(cfg Config) LatencyStats {
	// Custom optimized transport for high load
	transport := &http.Transport{
		MaxIdleConns:        10000,
		MaxIdleConnsPerHost: 5000,
		MaxConnsPerHost:     5000,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	methods := []string{"getMe"}
	if cfg.Scenario == "mixed" {
		methods = []string{"getMe", "getMyCommands", "getWebhookInfo", "getMyName"}
	} else if cfg.Scenario == "burst" {
		methods = []string{"getMe", "getMyDescription", "getWebhookInfo"}
	}

	var (
		totalReqs   atomic.Int64
		successReqs atomic.Int64
		errorReqs   atomic.Int64
		statusCodes sync.Map
	)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	resultsChan := make(chan float64, 500000)

	// Rate limiter ticker if enabled
	var limiter <-chan time.Time
	if cfg.RateLimit > 0 {
		interval := time.Second / time.Duration(cfg.RateLimit)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		limiter = ticker.C
	}

	startTime := time.Now()
	fmt.Printf("\n🚀 Launching stress load (%d workers, duration: %s)...\n", cfg.Concurrency, cfg.Duration)

	// Reporter goroutine
	reporterDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var lastCount int64
		lastTime := time.Now()

		for {
			select {
			case <-ticker.C:
				currentTotal := totalReqs.Load()
				currentSuccess := successReqs.Load()
				currentErrors := errorReqs.Load()
				elapsed := time.Since(lastTime).Seconds()
				intervalRPS := float64(currentTotal-lastCount) / elapsed
				lastCount = currentTotal
				lastTime = time.Now()

				fmt.Printf("   [+%.0fs] Sent: %d reqs | Current: %.1f RPS | Success: %d | Errors: %d\n",
					time.Since(startTime).Seconds(), currentTotal, intervalRPS, currentSuccess, currentErrors)
			case <-reporterDone:
				return
			}
		}
	}()

	// Workers
	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			idx := workerID

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if limiter != nil {
					select {
					case <-limiter:
					case <-ctx.Done():
						return
					}
				}

				method := methods[idx%len(methods)]
				idx++
				url := fmt.Sprintf("%s/bot%s/%s", cfg.ServerURL, cfg.Token, method)

				reqStart := time.Now()
				req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
				if err != nil {
					continue
				}

				totalReqs.Add(1)
				resp, err := client.Do(req)
				latencyMs := float64(time.Since(reqStart).Nanoseconds()) / 1e6

				if err != nil {
					errorReqs.Add(1)
					actual, _ := statusCodes.LoadOrStore(0, new(int64))
					atomic.AddInt64(actual.(*int64), 1)
					continue
				}

				// Drain body
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()

				actual, _ := statusCodes.LoadOrStore(resp.StatusCode, new(int64))
				atomic.AddInt64(actual.(*int64), 1)

				if resp.StatusCode == http.StatusOK {
					successReqs.Add(1)
				} else {
					errorReqs.Add(1)
				}

				select {
				case resultsChan <- latencyMs:
				default:
				}
			}
		}(w)
	}

	wg.Wait()
	close(reporterDone)
	close(resultsChan)
	actualDuration := time.Since(startTime)

	// Collect and analyze latencies
	latencies := make([]float64, 0, len(resultsChan))
	for l := range resultsChan {
		latencies = append(latencies, l)
	}
	sort.Float64s(latencies)

	statusMap := make(map[int]int64)
	statusCodes.Range(func(key, val interface{}) bool {
		statusMap[key.(int)] = atomic.LoadInt64(val.(*int64))
		return true
	})

	stats := LatencyStats{
		TotalReqs:   totalReqs.Load(),
		SuccessReqs: successReqs.Load(),
		ErrorReqs:   errorReqs.Load(),
		StatusCodes: statusMap,
		Latencies:   latencies,
		Duration:    actualDuration,
		RPS:         float64(totalReqs.Load()) / actualDuration.Seconds(),
	}

	if len(latencies) > 0 {
		var sum float64
		stats.Min = latencies[0]
		stats.Max = latencies[len(latencies)-1]
		for _, l := range latencies {
			sum += l
		}
		stats.Mean = sum / float64(len(latencies))
		stats.Median = percentile(latencies, 50)
		stats.P90 = percentile(latencies, 90)
		stats.P95 = percentile(latencies, 95)
		stats.P99 = percentile(latencies, 99)
	}

	return stats
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil((p / 100.0) * float64(len(sorted))))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func printResults(s LatencyStats, baseline, final PprofMetrics, cfg Config) {
	fmt.Println("\n================================================================================")
	fmt.Println("                            STRESS TEST RESULTS                                 ")
	fmt.Println("================================================================================")
	fmt.Printf(" Total Requests     : %d\n", s.TotalReqs)
	fmt.Printf(" Successful (200)   : %d (%.2f%%)\n", s.SuccessReqs, float64(s.SuccessReqs)/float64(max(1, s.TotalReqs))*100)
	fmt.Printf(" Failed/Errors      : %d (%.2f%%)\n", s.ErrorReqs, float64(s.ErrorReqs)/float64(max(1, s.TotalReqs))*100)
	fmt.Printf(" Actual Duration    : %.2f sec\n", s.Duration.Seconds())
	fmt.Printf(" Achieved Throughput: %.1f RPS\n", s.RPS)

	fmt.Println("\n--- HTTP Status Codes Breakdown ---")
	for code, count := range s.StatusCodes {
		if code == 0 {
			fmt.Printf("  Connection Errors / Timeouts: %d\n", count)
		} else {
			fmt.Printf("  HTTP %d: %d\n", code, count)
		}
	}

	fmt.Println("\n--- Latency Distribution (ms) ---")
	fmt.Printf("  Min    : %.2f ms\n", s.Min)
	fmt.Printf("  Mean   : %.2f ms\n", s.Mean)
	fmt.Printf("  Median : %.2f ms\n", s.Median)
	fmt.Printf("  P90    : %.2f ms\n", s.P90)
	fmt.Printf("  P95    : %.2f ms\n", s.P95)
	fmt.Printf("  P99    : %.2f ms\n", s.P99)
	fmt.Printf("  Max    : %.2f ms\n", s.Max)

	if cfg.PprofURL != "" {
		fmt.Println("\n--- Runtime & Memory Health Analysis ---")
		goroutineDelta := final.Goroutines - baseline.Goroutines
		heapDeltaMB := float64(int64(final.HeapInuse)-int64(baseline.HeapInuse)) / (1024 * 1024)

		fmt.Printf("  Goroutines Baseline : %d\n", baseline.Goroutines)
		fmt.Printf("  Goroutines Post-load: %d (Delta: %+d)\n", final.Goroutines, goroutineDelta)
		fmt.Printf("  Heap Inuse Baseline : %.2f MB\n", float64(baseline.HeapInuse)/(1024*1024))
		fmt.Printf("  Heap Inuse Post-load: %.2f MB (Delta: %+.2f MB)\n", float64(final.HeapInuse)/(1024*1024), heapDeltaMB)

		if goroutineDelta > 200 {
			fmt.Println("  ⚠️ WARNING: High goroutine delta detected! Check for goroutine leak.")
		} else {
			fmt.Println("  ✅ Goroutine stability: PASSED (No leak detected).")
		}

		if heapDeltaMB > 150 {
			fmt.Println("  ⚠️ WARNING: Significant heap growth observed!")
		} else {
			fmt.Println("  ✅ Memory stability: PASSED (No runaway memory consumption).")
		}
	}

	fmt.Println("================================================================================")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func runWebhookScenario(cfg Config, baseline PprofMetrics) {
	fmt.Println("\n🪝 Setting up local Webhook Sink on 127.0.0.1:9876...")
	var received atomic.Int64
	mockServer := &http.Server{
		Addr: "127.0.0.1:9876",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received.Add(1)
			w.WriteHeader(http.StatusOK)
		}),
	}
	go func() {
		_ = mockServer.ListenAndServe()
	}()
	defer mockServer.Close()

	// Call setWebhook
	setURL := fmt.Sprintf("%s/bot%s/setWebhook?url=http://127.0.0.1:9876/webhook", cfg.ServerURL, cfg.Token)
	resp, err := http.Get(setURL)
	if err != nil {
		fmt.Printf("❌ Failed to set webhook: %v\n", err)
		return
	}
	_ = resp.Body.Close()
	fmt.Println("✅ Webhook configured to http://127.0.0.1:9876/webhook")

	// Run stress test
	stats := runStressTest(cfg)

	// Clean up webhook
	delURL := fmt.Sprintf("%s/bot%s/deleteWebhook", cfg.ServerURL, cfg.Token)
	delResp, delErr := http.Get(delURL)
	if delErr == nil {
		_ = delResp.Body.Close()
	}

	fmt.Printf("\n📦 Total Webhook Updates Received by Sink: %d\n", received.Load())

	var final PprofMetrics
	if cfg.PprofURL != "" {
		fmt.Print("⏳ Waiting 3s for GC and cool-down...\n")
		time.Sleep(3 * time.Second)
		final = fetchPprof(cfg.PprofURL)
	}

	printResults(stats, baseline, final, cfg)
}

