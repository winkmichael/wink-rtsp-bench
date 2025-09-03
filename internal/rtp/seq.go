// Created by WINK Streaming (https://www.wink.co)
package rtp

import (
	"sync"
	"sync/atomic"
)

// SeqTracker tracks RTP sequence numbers and detects packet loss
type SeqTracker struct {
	mu          sync.Mutex
	initialized bool
	lastSeq     uint16
	totalLost   uint64
	totalPkts   uint64
	
	// Sequence number wrap detection
	cycles      uint32  // Number of sequence number cycles
	maxSeq      uint32  // Highest sequence number seen (with cycles)
	baseSeq     uint32  // First sequence number
	badSeq      uint32  // Last 'bad' sequence number + 1
	probation   int     // Packets left in probation
}

// NewSeqTracker creates a new sequence tracker
func NewSeqTracker() *SeqTracker {
	return &SeqTracker{
		probation: 0, // Start with no probation
	}
}

// Push processes a new RTP sequence number and returns packets lost
func (s *SeqTracker) Push(seq uint16) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		s.initSequence(seq)
		return 0
	}

	return s.updateSequence(seq)
}

// initSequence initializes tracking with the first sequence number
func (s *SeqTracker) initSequence(seq uint16) {
	s.baseSeq = uint32(seq)
	s.maxSeq = uint32(seq)
	s.lastSeq = seq
	s.badSeq = uint32(seq) + 1
	s.cycles = 0
	s.initialized = true
	s.totalPkts = 1
}

// updateSequence updates tracking with a new sequence number
func (s *SeqTracker) updateSequence(seq uint16) uint64 {
	udelta := uint16(seq - s.lastSeq)
	var lost uint64

	// Handle sequence number wraparound
	if udelta < 0x8000 {
		// Forward jump
		if udelta > 0 {
			// We may have lost packets
			if udelta > 1 {
				lost = uint64(udelta - 1)
				s.totalLost += lost
			}
			
			// Update max sequence with cycle tracking
			if seq < s.lastSeq {
				// Wrapped around
				s.cycles++
			}
			s.maxSeq = s.cycles<<16 | uint32(seq)
		}
		// else: duplicate packet (udelta == 0), ignore
	} else {
		// Large jump backwards or forwards
		if uint16(s.lastSeq-seq) < 0x8000 {
			// Actually a jump backwards - could be reordering
			// For now, treat as out of order and don't count as loss
		} else {
			// Very large forward jump (wrapped around)
			s.cycles++
			s.maxSeq = s.cycles<<16 | uint32(seq)
			
			// Calculate actual distance including wrap
			actualDelta := (0x10000 - uint32(s.lastSeq)) + uint32(seq)
			if actualDelta > 1 {
				lost = uint64(actualDelta - 1)
				s.totalLost += lost
			}
		}
	}

	s.lastSeq = seq
	s.totalPkts++
	
	return lost
}

// GetStats returns current statistics
func (s *SeqTracker) GetStats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return Stats{
		Packets:  s.totalPkts,
		Lost:     s.totalLost,
		LastSeq:  s.lastSeq,
		Cycles:   s.cycles,
	}
}

// Stats holds RTP statistics
type Stats struct {
	Packets  uint64
	Lost     uint64
	LastSeq  uint16
	Cycles   uint32
}

// Aggregator collects statistics from multiple trackers
type Aggregator struct {
	packets atomic.Uint64
	lost    atomic.Uint64
	bytes   atomic.Uint64
}

// NewAggregator creates a new statistics aggregator
func NewAggregator() *Aggregator {
	return &Aggregator{}
}

// AddPackets adds to packet count
func (a *Aggregator) AddPackets(n uint64) {
	if n > 0 {
		a.packets.Add(n)
	}
}

// AddLoss adds to loss count
func (a *Aggregator) AddLoss(n uint64) {
	if n > 0 {
		a.lost.Add(n)
	}
}

// AddBytes adds to byte count
func (a *Aggregator) AddBytes(n uint64) {
	if n > 0 {
		a.bytes.Add(n)
	}
}

// Snapshot returns current aggregate statistics
func (a *Aggregator) Snapshot() Snapshot {
	return Snapshot{
		Packets: a.packets.Load(),
		Lost:    a.lost.Load(),
		Bytes:   a.bytes.Load(),
	}
}

// Snapshot represents a point-in-time statistics snapshot
type Snapshot struct {
	Packets uint64
	Lost    uint64
	Bytes   uint64
}

// LossRate calculates the packet loss rate as a percentage
func (s Snapshot) LossRate() float64 {
	total := s.Packets + s.Lost
	if total == 0 {
		return 0
	}
	return float64(s.Lost) * 100.0 / float64(total)
}

// PacketRate calculates packets per second given a duration
func (s Snapshot) PacketRate(seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(s.Packets) / seconds
}

// Bitrate calculates bitrate in Mbps given a duration
func (s Snapshot) Bitrate(seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(s.Bytes) * 8 / seconds / 1_000_000
}