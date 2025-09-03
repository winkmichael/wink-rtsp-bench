# Multiple IP Addresses for 100k+ TCP Connections

## The 64k Port Limitation

A TCP connection is uniquely identified by a 4-tuple:
```
(source_ip, source_port, destination_ip, destination_port)
```

With a single source IP connecting to a single destination IP:port, you're limited by the ephemeral port range (typically ~64,000 ports).

## Why Multiple IPs Are Necessary

To achieve 100k+ concurrent connections to the same RTSP server:

| Connections | Min Source IPs | Ports per IP |
|------------|----------------|--------------|
| 50,000     | 1              | 50,000       |
| 100,000    | 2              | 50,000       |
| 500,000    | 8              | 62,500       |
| 1,000,000  | 16             | 62,500       |

## Configuration Methods

### Method 1: IP Aliases (Simplest)

Add multiple IPs to a single network interface:

```bash
# Add IP aliases to eth0
ip addr add 10.0.1.10/24 dev eth0
ip addr add 10.0.1.11/24 dev eth0
ip addr add 10.0.1.12/24 dev eth0
ip addr add 10.0.1.13/24 dev eth0

# Verify
ip addr show eth0
```

Persistent configuration in `/etc/network/interfaces`:

```bash
auto eth0
iface eth0 inet static
    address 10.0.1.10/24
    gateway 10.0.1.1

auto eth0:1
iface eth0:1 inet static
    address 10.0.1.11/24

auto eth0:2
iface eth0:2 inet static
    address 10.0.1.12/24
```

### Method 2: Multiple NICs

Use multiple physical or virtual network interfaces:

```bash
# Configure multiple interfaces
ip addr add 10.0.1.10/24 dev eth0
ip addr add 10.0.2.10/24 dev eth1
ip addr add 10.0.3.10/24 dev eth2
```

### Method 3: IPv6 (Vast Address Space)

With IPv6, you can use a /64 subnet with virtually unlimited addresses:

```bash
# Add IPv6 addresses
ip -6 addr add 2001:db8::10/64 dev eth0
ip -6 addr add 2001:db8::11/64 dev eth0
ip -6 addr add 2001:db8::12/64 dev eth0
```

## Binding to Specific Source IPs

### In the Benchmark Tool

Modify the RTSP client to bind to specific source IPs:

```go
// Example: Round-robin source IP selection
type IPPool struct {
    ips     []net.IP
    current atomic.Uint32
}

func (p *IPPool) Next() net.IP {
    idx := p.current.Add(1) % uint32(len(p.ips))
    return p.ips[idx]
}

func dialWithSourceIP(network, addr string, sourceIP net.IP) (net.Conn, error) {
    dialer := &net.Dialer{
        LocalAddr: &net.TCPAddr{
            IP: sourceIP,
        },
        Timeout: 5 * time.Second,
    }
    return dialer.Dial(network, addr)
}
```

### Command Line Usage

Future enhancement for the tool:

```bash
# Use specific source IPs
./wink-rtsp-bench \
    --url rtsp://server:554/stream \
    --readers 100000 \
    --bind-ips "10.0.1.10,10.0.1.11,10.0.1.12,10.0.1.13"

# Use IP range
./wink-rtsp-bench \
    --url rtsp://server:554/stream \
    --readers 100000 \
    --bind-cidr "10.0.1.0/28"  # Uses .1 through .14
```

## Network Architecture Examples

### Small Scale (50k connections)
```
┌─────────────┐
│   Client    │
│  10.0.1.10  │────────> RTSP Server
│ 50k ports   │         192.168.1.100:554
└─────────────┘
```

### Medium Scale (200k connections)
```
┌─────────────┐
│   Client    │
│  10.0.1.10  │────┐
│  10.0.1.11  │────┤
│  10.0.1.12  │────┼───> RTSP Server
│  10.0.1.13  │────┘    192.168.1.100:554
│ 50k each    │
└─────────────┘
```

### Large Scale (1M connections)
```
┌──────────────┐      ┌──────────────┐
│   Client 1   │      │   Client 2   │
│ 10.0.1.10-18 │──┐ ┌─│ 10.0.2.10-18 │
│ 500k total   │  │ │ │ 500k total   │
└──────────────┘  ↓ ↓ └──────────────┘
                  RTSP Server Farm
                  192.168.1.100-110
```

## Routing Considerations

Ensure your network can route all source IPs:

```bash
# Add routes if necessary
ip route add 10.0.1.0/24 dev eth0

# Enable IP forwarding if using NAT
echo 1 > /proc/sys/net/ipv4/ip_forward

# For policy routing with multiple interfaces
ip rule add from 10.0.1.0/24 table 100
ip route add default via 10.0.1.1 table 100
```

## ARP Considerations

With many IP aliases, ARP tables may need tuning:

```bash
# Increase ARP cache size
echo 8192 > /proc/sys/net/ipv4/neigh/default/gc_thresh1
echo 32768 > /proc/sys/net/ipv4/neigh/default/gc_thresh2
echo 65536 > /proc/sys/net/ipv4/neigh/default/gc_thresh3

# Reduce ARP cache timeout for faster recycling
echo 30 > /proc/sys/net/ipv4/neigh/default/gc_stale_time
```

## Verification Commands

```bash
# Check all IPs
ip addr show

# Test connectivity from each IP
for ip in 10.0.1.{10..20}; do
    ping -c 1 -I $ip rtsp-server.example.com
done

# Monitor connections per source IP
ss -tan | awk '{print $4}' | cut -d: -f1 | sort | uniq -c

# Check port usage per IP
for ip in 10.0.1.{10..13}; do
    echo "IP $ip: $(ss -tan | grep $ip | wc -l) connections"
done
```

## Cloud Provider Considerations

### AWS EC2
- Use Elastic Network Interfaces (ENIs)
- Each ENI supports multiple private IPs
- Instance type determines ENI/IP limits

### Google Cloud
- Use alias IP ranges
- Configure through VPC settings

### Azure
- Use multiple IP configurations per NIC
- Or multiple NICs per VM

## Performance Impact

Multiple IPs have minimal performance impact:
- Modern kernels handle IP aliases efficiently
- No additional routing overhead for local aliases
- Main bottleneck remains network bandwidth and CPU

## Example Script: Auto-Configure IPs

```bash
#!/bin/bash
# auto-configure-ips.sh

INTERFACE="eth0"
BASE_IP="10.0.1"
START=10
COUNT=8

echo "Configuring $COUNT IP addresses on $INTERFACE"

for i in $(seq 0 $((COUNT-1))); do
    IP="$BASE_IP.$((START + i))"
    echo "Adding $IP/24"
    ip addr add "$IP/24" dev "$INTERFACE"
done

echo "Configuration complete. Current IPs:"
ip addr show "$INTERFACE" | grep inet
```

## Troubleshooting

### "Cannot assign requested address"
- Verify IP is configured: `ip addr show`
- Check routing: `ip route`
- Ensure IP is on correct interface

### Uneven distribution across IPs
- Implement round-robin or random selection
- Monitor with: `ss -tan | awk '{print $4}' | cut -d: -f1 | sort | uniq -c`

### Connection failures with multiple IPs
- Check firewall rules for all source IPs
- Verify reverse path filtering: `sysctl net.ipv4.conf.all.rp_filter`

Created by WINK Streaming (https://www.wink.co)