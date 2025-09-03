// Created by WINK Streaming (https://www.wink.co)
package bench

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/winkstreaming/wink-rtsp-bench/internal/rtsp"
	"github.com/winkstreaming/wink-rtsp-bench/internal/rtp"
)

// RealWorldSimulator simulates realistic traffic patterns
type RealWorldSimulator struct {
	config      Config
	aggregator  *rtp.Aggregator
	
	// Statistics
	activeConnects  atomic.Int64
	totalConnects   atomic.Int64
	totalFailures   atomic.Int64
	targetConnects  atomic.Int64
	
	// Control
	connections map[string]*Connection
	connMu      sync.RWMutex
	wg          sync.WaitGroup
}

// Connection tracks individual connection state
type Connection struct {
	ID        string
	StartTime time.Time
	Client    *rtsp.Client
	Cancel    context.CancelFunc
}

// NewRealWorldSimulator creates a new real-world traffic simulator
func NewRealWorldSimulator(config Config, agg *rtp.Aggregator) *RealWorldSimulator {
	return &RealWorldSimulator{
		config:      config,
		aggregator:  agg,
		connections: make(map[string]*Connection),
	}
}

// Run executes the real-world simulation
func (s *RealWorldSimulator) Run(ctx context.Context) error {
	fmt.Printf("[%s] Starting real-world simulation\n", time.Now().Format("15:04:05"))
	fmt.Printf("[%s] Target: %d avg connections (±%.0f%% variance)\n", 
		time.Now().Format("15:04:05"), s.config.AvgConnections, s.config.Variance*100)
	
	// Start load pattern generator
	s.wg.Add(1)
	go s.generateLoadPattern(ctx)
	
	// Start connection manager
	s.wg.Add(1)
	go s.manageConnections(ctx)
	
	// Wait for completion
	<-ctx.Done()
	
	fmt.Printf("[%s] Shutting down simulation...\n", time.Now().Format("15:04:05"))
	s.wg.Wait()
	
	return nil
}

// generateLoadPattern creates realistic traffic patterns
func (s *RealWorldSimulator) generateLoadPattern(ctx context.Context) {
	defer s.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second) // Adjust load every 10 seconds
	defer ticker.Stop()
	
	// Initial target
	s.targetConnects.Store(int64(s.config.AvgConnections))
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.adjustTargetLoad()
		}
	}
}

// adjustTargetLoad simulates realistic load variations
func (s *RealWorldSimulator) adjustTargetLoad() {
	avg := float64(s.config.AvgConnections)
	variance := s.config.Variance
	
	// Generate patterns: peak hours, off-hours, gradual changes
	hour := time.Now().Hour()
	dayFactor := 1.0
	
	// Simulate daily patterns
	switch {
	case hour >= 9 && hour <= 11:   // Morning peak
		dayFactor = 1.2
	case hour >= 12 && hour <= 13:  // Lunch dip
		dayFactor = 0.9
	case hour >= 14 && hour <= 17:  // Afternoon steady
		dayFactor = 1.1
	case hour >= 18 && hour <= 22:  // Evening peak
		dayFactor = 1.3
	case hour >= 23 || hour <= 5:   // Night low
		dayFactor = 0.6
	default:
		dayFactor = 0.8
	}
	
	// Add random variation
	randomFactor := 1.0 + (rand.Float64()-0.5)*variance
	
	// Calculate new target
	newTarget := int64(avg * dayFactor * randomFactor)
	
	// Apply bounds
	minTarget := int64(avg * (1 - variance))
	maxTarget := int64(avg * (1 + variance))
	
	if newTarget < minTarget {
		newTarget = minTarget
	}
	if newTarget > maxTarget {
		newTarget = maxTarget
	}
	
	s.targetConnects.Store(newTarget)
	
	fmt.Printf("[%s] Load adjustment: target=%d active=%d\n",
		time.Now().Format("15:04:05"), newTarget, s.activeConnects.Load())
}

// manageConnections handles connection lifecycle
func (s *RealWorldSimulator) manageConnections(ctx context.Context) {
	defer s.wg.Done()
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			s.closeAllConnections()
			return
		case <-ticker.C:
			s.adjustConnections(ctx)
		}
	}
}

// adjustConnections adds or removes connections to meet target
func (s *RealWorldSimulator) adjustConnections(ctx context.Context) {
	current := s.activeConnects.Load()
	target := s.targetConnects.Load()
	
	diff := target - current
	
	if diff > 0 {
		// Add connections
		toAdd := diff
		if toAdd > 50 { // Limit burst additions
			toAdd = 50
		}
		
		for i := int64(0); i < toAdd; i++ {
			s.wg.Add(1)
			go s.addConnection(ctx)
		}
	} else if diff < 0 {
		// Remove connections
		toRemove := -diff
		if toRemove > 20 { // Limit burst removals
			toRemove = 20
		}
		
		s.removeConnections(toRemove)
	}
}

// addConnection creates a new RTSP connection
func (s *RealWorldSimulator) addConnection(ctx context.Context) {
	defer s.wg.Done()
	
	// Create unique ID
	connID := fmt.Sprintf("conn-%d-%d", time.Now().UnixNano(), rand.Int())
	
	// Create client
	client, err := rtsp.NewClient(s.config.URL, s.config.Transport, s.aggregator)
	if err != nil {
		s.totalFailures.Add(1)
		return
	}
	
	// Connect
	if err := client.Connect(); err != nil {
		s.totalFailures.Add(1)
		return
	}
	
	// Update stats
	s.totalConnects.Add(1)
	s.activeConnects.Add(1)
	
	// Random session duration (realistic variance)
	minDuration := 30 * time.Second
	maxDuration := s.config.Duration
	if maxDuration <= minDuration {
		maxDuration = 5 * time.Minute
	}
	
	durationRange := maxDuration - minDuration
	if durationRange <= 0 {
		durationRange = 4*time.Minute + 30*time.Second
	}
	
	duration := minDuration + time.Duration(rand.Int63n(int64(durationRange)))
	
	// Create context with timeout
	connCtx, cancel := context.WithTimeout(ctx, duration)
	
	// Store connection
	conn := &Connection{
		ID:        connID,
		StartTime: time.Now(),
		Client:    client,
		Cancel:    cancel,
	}
	
	s.connMu.Lock()
	s.connections[connID] = conn
	s.connMu.Unlock()
	
	// Run session
	if err := client.Run(connCtx); err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		s.totalFailures.Add(1)
	}
	
	// Cleanup
	s.connMu.Lock()
	delete(s.connections, connID)
	s.connMu.Unlock()
	
	s.activeConnects.Add(-1)
}

// removeConnections closes random connections
func (s *RealWorldSimulator) removeConnections(count int64) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	
	removed := int64(0)
	for id, conn := range s.connections {
		if removed >= count {
			break
		}
		
		// Close connection
		conn.Cancel()
		delete(s.connections, id)
		removed++
	}
}

// closeAllConnections shuts down all active connections
func (s *RealWorldSimulator) closeAllConnections() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	
	for _, conn := range s.connections {
		conn.Cancel()
	}
	
	// Clear map
	s.connections = make(map[string]*Connection)
}

// GetStats returns current statistics
func (s *RealWorldSimulator) GetStats() Stats {
	snapshot := s.aggregator.Snapshot()
	
	return Stats{
		ActiveConnects:  s.activeConnects.Load(),
		TotalConnects:   s.totalConnects.Load(),
		TotalFailures:   s.totalFailures.Load(),
		TargetConnects:  s.targetConnects.Load(),
		RTPPackets:      snapshot.Packets,
		RTPLoss:         snapshot.Lost,
		RTPBytes:        snapshot.Bytes,
	}
}

// LoadPattern represents different load patterns
type LoadPattern int

const (
	PatternSteady LoadPattern = iota
	PatternPeak
	PatternValley
	PatternSpike
	PatternGradual
)

// GeneratePattern creates specific load patterns for testing
func GeneratePattern(pattern LoadPattern, base int, amplitude float64) int {
	switch pattern {
	case PatternPeak:
		// Simulate peak traffic
		return base + int(float64(base)*amplitude)
	case PatternValley:
		// Simulate low traffic
		return base - int(float64(base)*amplitude)
	case PatternSpike:
		// Sudden spike
		if rand.Float64() < 0.1 { // 10% chance
			return base * 2
		}
		return base
	case PatternGradual:
		// Gradual sinusoidal change
		t := float64(time.Now().Unix())
		return base + int(float64(base)*amplitude*math.Sin(t/300))
	default:
		return base
	}
}