# WINK RTSP Benchmark

<div align="center">
  <img src="https://www.wink.co/img/winklogo_round.jpg" alt="WINK Logo" width="150"/>
</div>

High-performance RTSP load testing tool capable of simulating 10,000+ concurrent connections for benchmarking RTSP streaming servers, powered by Go's lightweight goroutines.

Created by [WINK Streaming](https://www.wink.co) for the streaming comunity.

## Why Go for Load Testing?

Go's goroutines make it uniquely qualified for massive load testing:
- **Lightweight Threads**: Each goroutine uses only ~2KB of stack memory vs 1MB+ for OS threads
- **Efficient Scheduling**: Go's runtime scheduler efficiently manages millions of goroutines
- **Built-in Concurrency**: Native channels and synchronization primitives
- **No Thread Pool Limits**: Spawn 100,000+ concurrent connections without complex thread management
- **Low Overhead**: M:N threading model maps goroutines to OS threads efficiently

## Features

- **Massive Scale**: Handle 10,000+ concurrent RTSP connections using Go goroutines (tested: 5,000 on 3-core, 10,000 on 8-core systems)
- **Transport Flexibility**: TCP interleaved (default) and UDP unicast support
- **Real Metrics**: Track actual RTP packet loss via sequence number analysis
- **Flexible Testing**: Sustained load and ramp-up testing modes
- **Production Ready**: Built for Debian 12 with comprehensive tuning guides

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/winkstreaming/wink-rtsp-bench.git
cd wink-rtsp-bench

# Build
go build -o wink-rtsp-bench ./cmd/wink-rtsp-bench

# Install globally (optional)
sudo cp wink-rtsp-bench /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/winkstreaming/wink-rtsp-bench/cmd/wink-rtsp-bench@latest
```

## Quick Start

### Basic Test (1,000 connections)

```bash
wink-rtsp-bench \
  --url rtsp://stream.example.com:554/live \
  --readers 1000 \
  --duration 1m \
  --rate 100/s
```

### TCP Interleaved Test (10,000 connections)

```bash
wink-rtsp-bench \
  --url rtsp://stream.example.com:554/live \
  --readers 10000 \
  --duration 5m \
  --rate 500/s \
  --transport tcp \
  --stats-interval 10s
```

### UDP Transport Test

```bash
wink-rtsp-bench \
  --url rtsp://stream.example.com:554/live \
  --readers 5000 \
  --duration 2m \
  --rate 200/s \
  --transport udp
```

### Aggressive Ramp Test

```bash
wink-rtsp-bench \
  --url rtsp://stream.example.com:554/live \
  --readers 50000 \
  --duration 10m \
  --rate 2000/s \
  --log json
```

### Real-World Production Testing (Recommended)

The `--real-world` mode combined with `--hours 168` (one week) provides the most realistic production test. This simulates actual daily usage patterns with variable load, connection churn, and network issues.

**3-Day Production Stress Test (3000-9000 concurrent users with 20% bad clients):**
```bash
wink-rtsp-bench \
  --url rtsp://production.server:554/live \
  --real-world \
  --avg-connections 6000 \
  --variance 0.5 \
  --hours 72 \
  --include-bad-clients \
  --bad-client-ratio 0.2 \
  --rate 100/s \
  --stats-interval 60s \
  --log json > stress-test-72h.log
```

**7-Day Endurance Test (simulating real deployment conditions):**
```bash
wink-rtsp-bench \
  --url rtsp://production.server:554/live \
  --real-world \
  --avg-connections 5000 \
  --variance 0.4 \
  --hours 168 \
  --include-bad-clients \
  --bad-client-ratio 0.15 \
  --rate 50/s \
  --stats-interval 5m \
  --log json > endurance-test-week.log
```

These tests truly put a server through its paces by:
- Varying load between 3000-9000 connections throughout the day
- Including 15-20% misbehaving clients (slow connections, garbage data, disconnects)
- Running continuously for extended periods to catch memory leaks and resource exhaustion
- Simulating real-world connection patterns (morning ramp-up, evening peaks, night valleys)

> **Pro Tip:** For testing beyond 64k connections, check out our [Multi-IP Configuration Guide](docs/multi-ip.md). With proper IP aliasing, you can achieve 100k, 500k, or even 1 million concurrent connections from a single machine!

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--url` | RTSP URL to test | `rtsp://127.0.0.1:8554/stream` |
| `--readers` | Number of concurrent connections | `1000` |
| `--duration` | How long each connection stays active | `5m` |
| `--hours` | Duration in hours (overrides --duration) | `0` |
| `--rate` | Connection rate (e.g., 1000/m, 50/s) | `600/m` |
| `--transport` | Transport protocol (tcp or udp) | `tcp` |
| `--stats-interval` | Statistics output interval | `5s` |
| `--log` | Output format (text or json) | `text` |
| `--real-world` | Enable real-world simulation mode | `false` |
| `--avg-connections` | Average connections for real-world mode | `500` |
| `--variance` | Load variance for real-world mode (0.0-1.0) | `0.3` |
| `--include-bad-clients` | Include misbehaving clients | `false` |
| `--bad-client-ratio` | Ratio of bad clients (0.0-1.0) | `0.1` |
| `--version` | Show version information | |

## Output Examples

### Text Format (Default)

```
╔══════════════════════════════════════════════════════════════╗
║     WINK Streaming RTSP Benchmark Tool v1.0.0              ║
║     Created by WINK Streaming (https://www.wink.co)         ║
╚══════════════════════════════════════════════════════════════╝

Configuration:
  Target URL:         rtsp://stream.example.com:554/live
  Concurrent Readers: 10000
  Connection Duration: 5m0s
  Connection Rate:    500.00/sec
  Transport:          TCP
  Stats Interval:     5s

[14:23:01] Starting benchmark: 10000 readers at 500.0/sec
[14:23:01] Spawned 100 connections
[14:23:02] Spawned 500 connections
[14:23:03] Spawned 1000 connections
[14:23:05] Active: 1000 | Connects: 1000 (+200.0/s) | Failures: 0 | RTP Packets: 125420 | Loss: 0
[14:23:10] Active: 2500 | Connects: 2500 (+300.0/s) | Failures: 2 | RTP Packets: 523180 | Loss: 3
[14:23:15] Active: 4200 | Connects: 4200 (+340.0/s) | Failures: 5 | RTP Packets: 1245630 | Loss: 12
```

### JSON Format

```json
{"time":"5s","active":1000,"connects":1000,"cps":200.0,"failures":0,"packets":125420,"loss":0}
{"time":"10s","active":2500,"connects":2500,"cps":300.0,"failures":2,"packets":523180,"loss":3}
{"time":"15s","active":4200,"connects":4200,"cps":340.0,"failures":5,"packets":1245630,"loss":12}
```

### Final Report

```
════════════════════════════════════════════════════════════════
BENCHMARK COMPLETE
────────────────────────────────────────────────────────────────
Duration:          5m0.234s
Total Connects:    10000
Total Failures:    23
Success Rate:      99.77%
RTP Packets:       45234521
RTP Loss:          234 (0.0005%)
Avg Connect Rate:  33.31/sec
════════════════════════════════════════════════════════════════
```

## System Tuning for High Concurrency

For tests exceeding 10,000 concurrent connections, system tuning is required. See [docs/linux-tuning.md](docs/linux-tuning.md) for detailed instructions.

### Quick Tuning (Current Session)

```bash
# Increase file descriptor limit
ulimit -n 1048576

# Apply kernel settings
sudo sysctl -w net.core.somaxconn=65535
sudo sysctl -w net.ipv4.ip_local_port_range="1024 65000"
sudo sysctl -w net.ipv4.tcp_fin_timeout=10
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
```

### Permanent Tuning

```bash
# Run the provided tuning script
sudo bash docs/tune-system.sh
```

## Scaling Beyond 64k Connections - The Ultimate Test

**Want to really push the limits?** A single IP address is limited to ~64,000 connections to the same destination due to port exhaustion. To achieve true massive scale testing (100k+ connections), you need multiple source IPs.

Our comprehensive [Multi-IP Configuration Guide](docs/multi-ip.md) covers:
- **Breaking the 64k barrier** with IP aliasing techniques
- **Achieving 1 million connections** using 16 source IPs
- **Practical examples** for 50k, 200k, and 1M connection setups
- **Cloud provider configurations** for AWS, GCP, and Azure
- **Auto-configuration scripts** for rapid deployment
- **Network architecture patterns** for different scales

If you're serious about stress testing at scale, this guide is essential reading. It transforms a single machine into a massive load generation platform capable of simulating entire data centers worth of clients.

## Architecture

```
┌─────────────────────────────────────────┐
│            Main Process                 │
│                                         │
│  ┌─────────────┐    ┌─────────────┐    │
│  │ Rate Limiter│───▶│  Spawner    │    │
│  └─────────────┘    └──────┬──────┘    │
│                            │            │
│         ┌──────────────────┴─────┐      │
│         ▼                        ▼      │
│  ┌──────────────┐         ┌──────────────┐
│  │ RTSP Client  │  ....   │ RTSP Client  │
│  │   (TCP/UDP)  │         │   (TCP/UDP)  │
│  └──────┬───────┘         └──────┬───────┘
│         │                        │      │
│         ▼                        ▼      │
│  ┌──────────────────────────────────┐  │
│  │      RTP Sequence Tracker        │  │
│  │    (Packet Loss Detection)       │  │
│  └──────────────┬───────────────────┘  │
│                 │                       │
│                 ▼                       │
│  ┌──────────────────────────────────┐  │
│  │      Statistics Aggregator       │  │
│  └──────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

## Performance Characteristics

### Critical: Bandwidth Reality Check

**Your network will saturate faster than you expect!** Testing with realistic bitrates is essential:

**Real-World Bandwidth Math:**
- **1080p HD Stream (6 Mbps)**: 170 connections = 1 Gbps saturated
- **720p Stream (3 Mbps)**: 340 connections = 1 Gbps saturated  
- **4K Stream (15 Mbps)**: 67 connections = 1 Gbps saturated
- **Low bitrate (500 kbps)**: 2,000 connections = 1 Gbps

**With a 10 Gbps NIC:**
- 1,600 × 6 Mbps HD streams = 10 Gbps saturated
- 3,300 × 3 Mbps 720p streams = 10 Gbps saturated
- 666 × 15 Mbps 4K streams = 10 Gbps saturated

> **Testing Best Practice:** Always test at your production bitrate! Testing 10,000 connections at 100 kbps is meaningless if your real streams are 6 Mbps. It's better to know you can handle 1,600 real HD connections than falsely believe you can handle 10,000 low-bitrate connections. Don't defeat the purpose of stress testing by using unrealistic bitrates.

### Resource Usage

| Connections | Memory | CPU Cores | Bandwidth @ 2 Mbps |
|-------------|--------|-----------|---------------------|
| 1,000       | ~100MB | 1-2       | 2 Gbps             |
| 10,000      | ~1GB   | 4-8       | 20 Gbps            |
| 50,000      | ~5GB   | 8-16      | 100 Gbps           |
| 100,000     | ~10GB  | 16-32     | 200 Gbps           |

### Bottlenecks

1. **Port Exhaustion**: Limited to ~64k ports per source IP
2. **File Descriptors**: Default limit is 1024, needs tuning
3. **CPU**: Connection handshakes are CPU intensive
4. **Memory**: Each connection maintains buffers
5. **Network**: Bandwidth scales linearly with connections

## Development

### Building from Source

```bash
# Clone repository
git clone https://github.com/winkstreaming/wink-rtsp-bench.git
cd wink-rtsp-bench

# Get dependencies
go mod download

# Build
go build -o wink-rtsp-bench ./cmd/wink-rtsp-bench

# Run tests
go test ./...
```

### Project Structure

```
wink-rtsp-bench/
├── cmd/
│   └── wink-rtsp-bench/
│       └── main.go           # CLI entry point
├── internal/
│   ├── bench/
│   │   └── runner.go         # Benchmark orchestrator
│   ├── rtsp/
│   │   └── client.go         # RTSP protocol implementation
│   └── rtp/
│       └── seq.go            # RTP sequence tracking
├── docs/
│   ├── linux-tuning.md       # System tuning guide
│   └── multi-ip.md           # Multiple IP configuration
├── go.mod
├── go.sum
└── README.md
```

## Troubleshooting

### "Too many open files"

Increase file descriptor limits:
```bash
ulimit -n 1048576
```

### "Cannot assign requested address"

Port exhaustion. Solutions:
- Reduce connection count
- Add more source IPs (see docs/multi-ip.md)
- Decrease tcp_fin_timeout

### High packet loss

Possible causes:
- Network congestion
- Insufficient receive buffers
- CPU bottleneck in packet processing

### Connection failures

Check:
- RTSP server capacity
- Network path MTU
- Firewall rules
- Server connection limits

## Contributing

Contributions are welcome! Please read our contributing guidelines and submit pull requests to our repository.

## License

Apache License 2.0. See LICENSE file for details.

## Support

- GitHub Issues: [github.com/winkstreaming/wink-rtsp-bench/issues](https://github.com/winkstreaming/wink-rtsp-bench/issues)
- Email: support@wink.co

## About WINK Streaming

WINK Streaming provides secure cloud-based video sharing solutions for government, public safety, and enterprise organizations. Our platform enables organizations to securely distribute video feeds both internally and with the public while maintaining complete control through role-based access, comprehensive authentication, and audit trails.

We specialize in mission-critical video infrastructure for transportation, smart cities, law enforcement, and emergency managment, with native integrations for security platforms and global content delivery, and work with all cloud providers and offer hosting, SaaS, Virtual and hardware deployments.

Visit [https://www.wink.co](https://www.wink.co) for more information.

---

Created by WINK Streaming • High-Performance Streaming Solutions