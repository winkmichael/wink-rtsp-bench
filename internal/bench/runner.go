// Created by WINK Streaming (https://www.wink.co)
package bench

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/winkstreaming/wink-rtsp-bench/internal/rtsp"
	"github.com/winkstreaming/wink-rtsp-bench/internal/rtp"
	"golang.org/x/time/rate"
)

// Config holds benchmark configuration
type Config struct {
	URL           string
	Readers       int
	Duration      time.Duration
	Rate          float64 // connections per second
	Transport     string
	StatsInterval time.Duration
	LogFormat     string
	RealWorld     bool    // Enable real-world simulation
	AvgConnections int    // Average connections for real-world mode
	Variance      float64 // Load variance (0.0-1.0)
	IncludeBadClients bool    // Include misbehaving clients
	BadClientRatio    float64 // Ratio of bad clients (0.0-1.0)
}

// Runner orchestrates the benchmark
type Runner struct {
	config     Config
	aggregator *rtp.Aggregator
	
	// Statistics
	activeConnects  atomic.Int64
	totalConnects   atomic.Int64
	totalFailures   atomic.Int64
	connectLatency  atomic.Int64 // cumulative milliseconds
	connectCount    atomic.Int64
	badClients      atomic.Int64 // Number of bad clients spawned
	badClientTypes  sync.Map     // Track types of bad clients
	
	// Latency tracking
	latencies      []float64
	latenciesMu    sync.Mutex
	minLatency     atomic.Int64
	maxLatency     atomic.Int64
	
	// Control
	limiter    *rate.Limiter
	semaphore  chan struct{}
	wg         sync.WaitGroup
}

// NewRunner creates a new benchmark runner
func NewRunner(config Config, agg *rtp.Aggregator) *Runner {
	// Create rate limiter - allow burst of 10 connections
	burst := 10
	if config.Rate > 100 {
		burst = int(config.Rate / 10)
	}
	if burst > 100 {
		burst = 100
	}
	
	// Semaphore to limit concurrent connection attempts
	// This prevents overwhelming the system during ramp-up
	maxConcurrent := 10000
	if config.Readers > 10000 {
		maxConcurrent = config.Readers / 10
		if maxConcurrent > 50000 {
			maxConcurrent = 50000
		}
	}
	
	r := &Runner{
		config:     config,
		aggregator: agg,
		limiter:    rate.NewLimiter(rate.Limit(config.Rate), burst),
		semaphore:  make(chan struct{}, maxConcurrent),
		latencies:  make([]float64, 0, 1000),
	}
	r.minLatency.Store(99999999)
	r.maxLatency.Store(0)
	return r
}

// Run executes the benchmark
func (r *Runner) Run(ctx context.Context) error {
	// Check if real-world mode is enabled
	if r.config.RealWorld {
		simulator := NewRealWorldSimulator(r.config, r.aggregator)
		return simulator.Run(ctx)
	}
	
	fmt.Printf("[%s] Starting benchmark: %d readers at %.1f/sec\n",
		time.Now().Format("15:04:05"), r.config.Readers, r.config.Rate)
	
	// Create a context that we can cancel
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	// Start connection spawner
	r.wg.Add(1)
	go r.spawnConnections(runCtx)
	
	// Wait for completion or cancellation
	<-runCtx.Done()
	
	// Wait for all connections to finish
	fmt.Printf("[%s] Waiting for connections to close...\n", time.Now().Format("15:04:05"))
	r.wg.Wait()
	
	return nil
}

// spawnConnections creates connections at the configured rate
func (r *Runner) spawnConnections(ctx context.Context) {
	defer r.wg.Done()
	
	connectionsCreated := 0
	lastCheck := time.Now()
	lastFailures := int64(0)
	
	for connectionsCreated < r.config.Readers {
		// Check for cancellation
		if ctx.Err() != nil {
			return
		}
		
		// Adaptive rate limiting - check every 10 connections
		if connectionsCreated > 0 && connectionsCreated%10 == 0 {
			now := time.Now()
			if now.Sub(lastCheck) > 2*time.Second {
				currentFailures := r.totalFailures.Load()
				failureDelta := currentFailures - lastFailures
				totalDelta := int64(10)
				
				// If failure rate > 20%, slow down
				if failureDelta > totalDelta/5 {
					// Reduce rate by 50%
					newRate := r.limiter.Limit() / 2
					if newRate < 1 {
						newRate = 1
					}
					r.limiter.SetLimit(newRate)
					fmt.Printf("[%s] High failure rate detected (%d/%d), reducing rate to %.1f/s\n",
						time.Now().Format("15:04:05"), failureDelta, totalDelta, float64(newRate))
				} else if failureDelta == 0 && r.limiter.Limit() < rate.Limit(r.config.Rate) {
					// If no failures and we're below target rate, increase by 20%
					newRate := r.limiter.Limit() * 1.2
					if newRate > rate.Limit(r.config.Rate) {
						newRate = rate.Limit(r.config.Rate)
					}
					r.limiter.SetLimit(newRate)
					fmt.Printf("[%s] Success rate good, increasing rate to %.1f/s\n",
						time.Now().Format("15:04:05"), float64(newRate))
				}
				
				lastCheck = now
				lastFailures = currentFailures
			}
		}
		
		// Rate limit
		if err := r.limiter.Wait(ctx); err != nil {
			return
		}
		
		// Acquire semaphore slot
		select {
		case r.semaphore <- struct{}{}:
		case <-ctx.Done():
			return
		}
		
		// Spawn connection - decide if it should be a bad client
		r.wg.Add(1)
		if r.config.IncludeBadClients && rand.Float64() < r.config.BadClientRatio {
			go r.runBadClient(ctx)
		} else {
			go r.runConnection(ctx)
		}
		
		connectionsCreated++
		
		// Log progress every 100 connections initially, then every 1000
		if connectionsCreated <= 1000 && connectionsCreated%100 == 0 {
			fmt.Printf("[%s] Spawned %d connections\n", 
				time.Now().Format("15:04:05"), connectionsCreated)
		} else if connectionsCreated%1000 == 0 {
			fmt.Printf("[%s] Spawned %d connections\n",
				time.Now().Format("15:04:05"), connectionsCreated)
		}
	}
	
	fmt.Printf("[%s] Finished spawning %d connections\n",
		time.Now().Format("15:04:05"), connectionsCreated)
}

// runConnection manages a single RTSP connection
func (r *Runner) runConnection(ctx context.Context) {
	defer r.wg.Done()
	defer func() { <-r.semaphore }() // Release semaphore slot
	
	// Retry logic for connection establishment
	const maxRetries = 3
	var client *rtsp.Client
	var err error
	var connectDuration time.Duration
	
	for retry := 0; retry < maxRetries; retry++ {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return
		}
		
		// Create client
		startTime := time.Now()
		client, err = rtsp.NewClient(r.config.URL, r.config.Transport, r.aggregator)
		if err != nil {
			if retry == maxRetries-1 {
				r.totalFailures.Add(1)
				return
			}
			// Exponential backoff: 100ms, 200ms, 400ms
			time.Sleep(time.Duration(100*(1<<retry)) * time.Millisecond)
			continue
		}
		
		// Connect
		if err = client.Connect(); err != nil {
			if retry == maxRetries-1 {
				r.totalFailures.Add(1)
				return
			}
			// Exponential backoff
			time.Sleep(time.Duration(100*(1<<retry)) * time.Millisecond)
			continue
		}
		
		// Success!
		connectDuration = time.Since(startTime)
		break
	}
	
	// Track connection time
	latencyMs := connectDuration.Milliseconds()
	r.connectLatency.Add(int64(latencyMs))
	r.connectCount.Add(1)
	
	// Update min/max
	for {
		oldMin := r.minLatency.Load()
		if int64(latencyMs) >= oldMin || r.minLatency.CompareAndSwap(oldMin, int64(latencyMs)) {
			break
		}
	}
	for {
		oldMax := r.maxLatency.Load()
		if int64(latencyMs) <= oldMax || r.maxLatency.CompareAndSwap(oldMax, int64(latencyMs)) {
			break
		}
	}
	
	// Store for percentile calculation
	r.latenciesMu.Lock()
	if len(r.latencies) < 10000 { // Limit memory usage
		r.latencies = append(r.latencies, float64(latencyMs))
	}
	r.latenciesMu.Unlock()
	
	// Update counters
	r.totalConnects.Add(1)
	r.activeConnects.Add(1)
	defer r.activeConnects.Add(-1)
	
	// Create context with duration timeout
	runCtx, cancel := context.WithTimeout(ctx, r.config.Duration)
	defer cancel()
	
	// Run the session
	if err := client.Run(runCtx); err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		// Only count as failure if it's not a normal timeout/cancel
		r.totalFailures.Add(1)
	}
}

// runBadClient manages a single misbehaving RTSP client
func (r *Runner) runBadClient(ctx context.Context) {
	defer r.wg.Done()
	defer func() { <-r.semaphore }() // Release semaphore slot
	
	// Create bad client
	badClient := rtsp.NewBadClient(r.config.URL)
	
	// Track bad client statistics
	r.badClients.Add(1)
	r.activeConnects.Add(1)
	defer r.activeConnects.Add(-1)
	
	// Track bad client type
	typeName := badClient.GetTypeName()
	if count, ok := r.badClientTypes.Load(typeName); ok {
		r.badClientTypes.Store(typeName, count.(int64)+1)
	} else {
		r.badClientTypes.Store(typeName, int64(1))
	}
	
	// Create context with duration timeout
	runCtx, cancel := context.WithTimeout(ctx, r.config.Duration)
	defer cancel()
	
	// Run the bad client (errors are expected and ignored)
	_ = badClient.Run(runCtx)
}

// Stats represents current benchmark statistics
type Stats struct {
	ActiveConnects  int64
	TotalConnects   int64
	TotalFailures   int64
	TargetConnects  int64   // For real-world mode
	AvgConnectTime  float64 // milliseconds
	MinConnectTime  float64 // milliseconds
	MaxConnectTime  float64 // milliseconds
	P95ConnectTime  float64 // milliseconds
	RTPPackets      uint64
	RTPLoss         uint64
	RTPBytes        uint64
	BadClients      int64   // Number of bad clients
	BadClientTypes  map[string]int64 // Count by type
}

// GetStats returns current statistics
func (r *Runner) GetStats() Stats {
	snapshot := r.aggregator.Snapshot()
	
	// Calculate average connection time
	var avgConnect float64
	count := r.connectCount.Load()
	if count > 0 {
		avgConnect = float64(r.connectLatency.Load()) / float64(count)
	}
	
	// Calculate percentiles
	var p95 float64
	r.latenciesMu.Lock()
	if len(r.latencies) > 0 {
		p95 = calculatePercentile(r.latencies, 95)
	}
	r.latenciesMu.Unlock()
	
	minLat := float64(r.minLatency.Load())
	if minLat == 99999999 {
		minLat = 0
	}
	
	// Collect bad client types
	badClientTypes := make(map[string]int64)
	r.badClientTypes.Range(func(key, value interface{}) bool {
		badClientTypes[key.(string)] = value.(int64)
		return true
	})
	
	return Stats{
		ActiveConnects:  r.activeConnects.Load(),
		TotalConnects:   r.totalConnects.Load(),
		TotalFailures:   r.totalFailures.Load(),
		AvgConnectTime:  avgConnect,
		MinConnectTime:  minLat,
		MaxConnectTime:  float64(r.maxLatency.Load()),
		P95ConnectTime:  p95,
		RTPPackets:      snapshot.Packets,
		RTPLoss:         snapshot.Lost,
		RTPBytes:        snapshot.Bytes,
		BadClients:      r.badClients.Load(),
		BadClientTypes:  badClientTypes,
	}
}

// PrintStats prints formatted statistics
func (r *Runner) PrintStats() {
	stats := r.GetStats()
	lossRate := float64(0)
	if stats.RTPPackets > 0 {
		lossRate = float64(stats.RTPLoss) * 100.0 / float64(stats.RTPPackets+stats.RTPLoss)
	}
	
	fmt.Printf("Active: %d | Total: %d | Failed: %d | Avg Connect: %.1fms | Packets: %d | Loss: %.2f%%\n",
		stats.ActiveConnects,
		stats.TotalConnects,
		stats.TotalFailures,
		stats.AvgConnectTime,
		stats.RTPPackets,
		lossRate,
	)
}

// calculatePercentile calculates the nth percentile of a slice of values
func calculatePercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	// Create a copy to avoid modifying original
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	index := (percentile / 100) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1
	
	if upper >= len(sorted) {
		return sorted[lower]
	}
	
	// Linear interpolation
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}