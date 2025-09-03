# Linux Tuning for 100k+ Concurrent RTSP Connections

This guide covers kernel and system tuning for Debian 12 to support 100,000+ concurrent RTSP connections.

## Quick Setup Script

```bash
#!/bin/bash
# Save as tune-system.sh and run with sudo

# Increase file descriptor limits for current session
ulimit -n 1048576

# Create systemd service override for your service
mkdir -p /etc/systemd/system/wink-rtsp-bench.service.d
cat > /etc/systemd/system/wink-rtsp-bench.service.d/limits.conf <<EOF
[Service]
LimitNOFILE=1048576
LimitNPROC=32768
EOF

# Apply kernel tuning
cat > /etc/sysctl.d/99-wink-rtsp-bench.conf <<'EOF'
# File Descriptors
fs.file-max = 2097152
fs.nr_open = 1048576

# Network Core
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 250000
net.core.rmem_default = 268435456
net.core.rmem_max = 268435456
net.core.wmem_default = 268435456
net.core.wmem_max = 268435456
net.core.optmem_max = 134217728

# TCP Settings
net.ipv4.tcp_rmem = 4096 131072 268435456
net.ipv4.tcp_wmem = 4096 65536 268435456
net.ipv4.tcp_mem = 786432 1048576 268435456
net.ipv4.tcp_max_syn_backlog = 262144
net.ipv4.tcp_syn_retries = 3
net.ipv4.tcp_synack_retries = 3
net.ipv4.tcp_fin_timeout = 10
net.ipv4.tcp_keepalive_time = 60
net.ipv4.tcp_keepalive_intvl = 10
net.ipv4.tcp_keepalive_probes = 6
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_abort_on_overflow = 1
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_timestamps = 1
net.ipv4.tcp_syncookies = 0

# Port Range (critical for client connections)
net.ipv4.ip_local_port_range = 1024 65000

# Connection Tracking (if using netfilter)
net.netfilter.nf_conntrack_max = 1048576
net.nf_conntrack_max = 1048576
net.netfilter.nf_conntrack_tcp_timeout_established = 1800
net.netfilter.nf_conntrack_tcp_timeout_time_wait = 30
net.netfilter.nf_conntrack_tcp_timeout_close_wait = 30
net.netfilter.nf_conntrack_tcp_timeout_fin_wait = 30

# UDP Buffer Sizes (for UDP transport)
net.core.rmem_default = 268435456
net.core.rmem_max = 536870912
net.core.wmem_default = 268435456
net.core.wmem_max = 536870912
net.ipv4.udp_rmem_min = 131072
net.ipv4.udp_wmem_min = 131072
EOF

# Apply sysctl settings
sysctl --system

# Update limits.conf for all users
cat >> /etc/security/limits.conf <<EOF

# WINK RTSP Benchmark limits
* soft nofile 1048576
* hard nofile 1048576
* soft nproc 32768
* hard nproc 32768
root soft nofile 1048576
root hard nofile 1048576
EOF

# Update PAM to use limits
grep -q "pam_limits.so" /etc/pam.d/common-session || \
echo "session required pam_limits.so" >> /etc/pam.d/common-session

echo "System tuning complete. Please reboot for all changes to take effect."
```

## Detailed Explanations

### File Descriptors

Each TCP connection requires a file descriptor. Linux defaults are too low for high-concurrency testing.

```bash
# Check current limits
ulimit -n                    # Soft limit
ulimit -Hn                   # Hard limit
cat /proc/sys/fs/file-max    # System-wide limit

# Monitor usage
lsof | wc -l                 # Count open files
cat /proc/sys/fs/file-nr     # Used, free, max
```

### Network Stack

#### Core Settings
- `somaxconn`: Maximum queued connections waiting for accept()
- `netdev_max_backlog`: Maximum packets queued for processing
- `rmem_max/wmem_max`: Maximum socket buffer sizes

#### TCP Tuning
- `tcp_fin_timeout`: Time to keep FIN-WAIT-2 state (reduce for faster port recycling)
- `tcp_tw_reuse`: Reuse TIME-WAIT sockets for new connections
- `tcp_max_syn_backlog`: Maximum queued SYN requests
- `tcp_abort_on_overflow`: Reset connections when backlog is full

#### Port Range
- Default range gives ~64k ports per IP address
- For >64k connections to same destination, need multiple source IPs

### Memory Calculations

For 100k connections with 256KB buffers each:
```
100,000 * 256KB * 2 (send+receive) = 51.2GB RAM minimum
```

Recommended system RAM: 128GB+ for 100k connections

### Monitoring During Tests

```bash
# Watch connection counts
watch -n1 'ss -s'

# Monitor by state
ss -tan | awk '{print $1}' | sort | uniq -c

# Check port usage
ss -tan | awk '{print $4}' | cut -d: -f2 | sort -n | uniq | wc -l

# Monitor network interrupts
watch -n1 'cat /proc/interrupts | grep -E "eth|eno|ens"'

# Check for packet drops
ip -s link show
netstat -s | grep -i drop

# Monitor TCP retransmissions
netstat -s | grep -i retrans
```

## CPU Optimization

### IRQ Affinity

Distribute network interrupts across CPU cores:

```bash
# Find network IRQs
cat /proc/interrupts | grep -E "eth0|eno1"

# Set affinity (example for IRQ 24 to CPU 0-3)
echo 0-3 > /proc/irq/24/smp_affinity_list

# Or use irqbalance service
systemctl enable --now irqbalance
```

### CPU Frequency Scaling

For consistent performance:

```bash
# Set performance governor
for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    echo performance > $cpu
done

# Disable CPU idle states for lowest latency
for cpu in /sys/devices/system/cpu/cpu*/cpuidle/state*/disable; do
    echo 1 > $cpu
done
```

## Troubleshooting

### Common Issues

1. **"Too many open files"**
   - Check ulimit -n
   - Verify /etc/security/limits.conf
   - Restart session after changes

2. **"Cannot assign requested address"**
   - Port exhaustion - check ip_local_port_range
   - TIME-WAIT accumulation - reduce tcp_fin_timeout

3. **"No buffer space available"**
   - Increase rmem_max/wmem_max
   - Check available RAM

4. **High CPU in softirq**
   - Distribute IRQs across cores
   - Enable RPS/RFS for packet steering

### Performance Validation

Test your tuning with smaller scales first:

```bash
# Test 1k connections
./wink-rtsp-bench --url rtsp://localhost:8554/test --readers 1000 --rate 100/s

# Test 10k connections
./wink-rtsp-bench --url rtsp://localhost:8554/test --readers 10000 --rate 500/s

# Test 50k connections
./wink-rtsp-bench --url rtsp://localhost:8554/test --readers 50000 --rate 1000/s
```

## Hardware Recommendations

For 100k concurrent RTSP connections:

- **CPU**: 16+ cores (Intel Xeon or AMD EPYC)
- **RAM**: 128GB minimum, 256GB recommended
- **Network**: 10Gbps NIC minimum
- **Storage**: NVMe SSD for logging/metrics

## Additional Resources

- [Linux TCP Tuning Guide](https://www.kernel.org/doc/Documentation/networking/ip-sysctl.txt)
- [Debian Network Configuration](https://wiki.debian.org/NetworkConfiguration)
- [High Performance Browser Networking](https://hpbn.co/)

Created by WINK Streaming (https://www.wink.co)