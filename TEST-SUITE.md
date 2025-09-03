# WINK RTSP Benchmark Test Suite

Created by WINK Streaming (https://www.wink.co)

## Quick Start

```bash
# Build the tool
cd /opt/wink-rtsp-bench
go build ./cmd/wink-rtsp-bench

# Run quick test
./wink-rtsp-bench --url rtsp://localhost:8554/bunny --readers 10 --duration 30s
```

## Test Scenarios

### 1. Basic Connectivity Tests

```bash
# 10 concurrent connections
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 10 \
  --duration 30s \
  --rate 2/s

# 100 concurrent connections  
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 100 \
  --duration 1m \
  --rate 10/s

# 1000 concurrent connections
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 1000 \
  --duration 2m \
  --rate 50/s
```

### 2. Real-World Simulation

Simulates realistic traffic patterns with variable load:

```bash
# Small deployment (50 avg users)
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --real-world \
  --avg-connections 50 \
  --variance 0.3 \
  --duration 10m

# Medium deployment (500 avg users)
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --real-world \
  --avg-connections 500 \
  --variance 0.4 \
  --duration 1h

# Large deployment (5000 avg users)
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --real-world \
  --avg-connections 5000 \
  --variance 0.5 \
  --hours 24
```

### 3. Endurance Tests

Long-running tests for stability validation:

```bash
# 1 hour test
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 100 \
  --hours 1 \
  --rate 5/s \
  --stats-interval 30s

# 24 hour test
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --real-world \
  --avg-connections 500 \
  --variance 0.4 \
  --hours 24 \
  --stats-interval 1m

# 48 hour test
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --real-world \
  --avg-connections 1000 \
  --variance 0.5 \
  --hours 48 \
  --stats-interval 5m

# 72 hour test
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --real-world \
  --avg-connections 2000 \
  --variance 0.5 \
  --hours 72 \
  --stats-interval 10m
```

### 4. Burst Tests

Test server behavior under sudden load:

```bash
# Rapid connection burst
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 500 \
  --duration 30s \
  --rate 100/s

# Sustained high rate
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 1000 \
  --duration 5m \
  --rate 200/s
```

### 5. UDP Transport Tests

```bash
# UDP mode test
./wink-rtsp-bench \
  --url rtsp://localhost:8554/bunny \
  --readers 100 \
  --duration 2m \
  --transport udp \
  --rate 10/s
```

## Automated Test Scripts

### Run All Basic Tests

```bash
# Run complete test suite
./scripts/test-scenarios.sh
```

### Endurance Test Management

```bash
# Start 24-hour test
./scripts/endurance-tests.sh start 24h

# Check status
./scripts/endurance-tests.sh status

# Stop all tests
./scripts/endurance-tests.sh stop
```

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--url` | RTSP URL to test | `rtsp://127.0.0.1:8554/stream` |
| `--readers` | Number of concurrent connections | `1000` |
| `--duration` | Connection duration | `5m` |
| `--hours` | Duration in hours (overrides --duration) | `0` |
| `--rate` | Connection rate (e.g., 100/s, 1000/m) | `600/m` |
| `--transport` | Transport protocol (tcp/udp) | `tcp` |
| `--real-world` | Enable real-world simulation | `false` |
| `--avg-connections` | Average connections for real-world mode | `500` |
| `--variance` | Load variance (0.0-1.0) | `0.3` |
| `--stats-interval` | Statistics output interval | `5s` |
| `--log` | Output format (text/json) | `text` |

## Real-World Simulation Features

The real-world simulator includes:

1. **Daily Traffic Patterns**
   - Morning peak (9-11 AM): 120% of average
   - Lunch dip (12-1 PM): 90% of average  
   - Afternoon steady (2-5 PM): 110% of average
   - Evening peak (6-10 PM): 130% of average
   - Night low (11 PM-5 AM): 60% of average

2. **Random Variations**
   - Configurable variance (default 30%)
   - Gradual load changes
   - Occasional traffic spikes

3. **Connection Lifecycle**
   - Variable session durations
   - Realistic connect/disconnect patterns
   - Automatic load rebalancing

## Performance Metrics

The tool tracks:
- Active connections
- Total connections established
- Connection failures
- Average connection latency
- RTP packets received
- RTP packet loss
- Data throughput

## Output Formats

### Text Format (Default)
```
[15s] Active: 450 | Connects: 500 (+50.0/s) | Failures: 5 | RTP Packets: 125420 | Loss: 12
```

### JSON Format
```json
{"time":"15s","active":450,"connects":500,"cps":50.0,"failures":5,"packets":125420,"loss":12}
```

## System Requirements

### For 100 connections
- CPU: 2 cores
- RAM: 512MB
- Network: 10 Mbps

### For 1,000 connections  
- CPU: 4 cores
- RAM: 2GB
- Network: 100 Mbps
- Tuning: `ulimit -n 10000`

### For 10,000 connections
- CPU: 8 cores
- RAM: 8GB
- Network: 1 Gbps
- Tuning: See `docs/linux-tuning.md`

### For 100,000 connections
- CPU: 16+ cores
- RAM: 32GB
- Network: 10 Gbps
- Tuning: Full kernel optimization
- Multiple source IPs required

## Troubleshooting

### Connection Failures

If all connections fail immediately:
1. Check RTSP server is running: `systemctl status mediamtx`
2. Verify stream exists: `ffprobe rtsp://localhost:8554/bunny`
3. Check firewall rules
4. Review server logs: `journalctl -u mediamtx -f`

### Performance Issues

For poor performance:
1. Increase file descriptors: `ulimit -n 100000`
2. Tune TCP stack: See `docs/linux-tuning.md`
3. Use multiple client IPs for >64k connections
4. Monitor CPU/memory usage: `htop`
5. Check network bandwidth: `iftop`

### Real-World Mode Issues

If real-world simulation behaves incorrectly:
1. Verify variance is between 0.0 and 1.0
2. Check average connections is reasonable
3. Ensure duration is longer than 1 minute
4. Monitor target vs actual connections in output

## Test Result Analysis

### Successful Test Indicators
- Connection success rate >95%
- RTP packet loss <0.1%
- Stable active connection count
- No memory leaks over time

### Warning Signs
- High connection failure rate
- Increasing RTP packet loss
- Memory usage growing unbounded
- CPU consistently at 100%

## Best Practices

1. **Start Small**: Begin with 10-100 connections
2. **Gradual Scaling**: Increase by 10x each test
3. **Monitor Resources**: Watch CPU, memory, network
4. **Long Tests**: Run 24+ hour tests for production validation
5. **Real-World Mode**: Use for realistic traffic patterns
6. **Multiple Runs**: Test multiple times for consistency

Created by WINK Streaming (https://www.wink.co)