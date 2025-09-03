#!/bin/bash
# Created by WINK Streaming (https://www.wink.co)

BENCH="/opt/wink-rtsp-bench/wink-rtsp-bench"
URL="rtsp://localhost:8554/bunny"
LOG_DIR="/opt/wink-rtsp-bench/test-results"

# Create log directory
mkdir -p "$LOG_DIR"

# Function to run test and save results
run_test() {
    local name=$1
    local args=$2
    local duration=$3
    
    echo "════════════════════════════════════════════════════════════════"
    echo "Starting test: $name"
    echo "Duration: $duration"
    echo "────────────────────────────────────────────────────────────────"
    
    log_file="$LOG_DIR/${name}_$(date +%Y%m%d_%H%M%S).log"
    
    timeout "$duration" $BENCH $args 2>&1 | tee "$log_file"
    
    echo "Test completed. Results saved to: $log_file"
    echo ""
    sleep 5
}

# Test scenarios
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║     WINK RTSP Benchmark Test Suite                          ║"
echo "║     Created by WINK Streaming (https://www.wink.co)         ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# 1. Quick connectivity test
run_test "quick_test" \
    "--url $URL --readers 5 --duration 10s --rate 1/s" \
    "15s"

# 2. 10 concurrent connections
run_test "10_concurrent" \
    "--url $URL --readers 10 --duration 30s --rate 2/s" \
    "35s"

# 3. 100 concurrent connections
run_test "100_concurrent" \
    "--url $URL --readers 100 --duration 1m --rate 10/s" \
    "65s"

# 4. Real-world simulation (small)
run_test "realworld_small" \
    "--url $URL --real-world --avg-connections 50 --variance 0.3 --duration 2m --rate 5/s" \
    "125s"

# 5. Burst test
run_test "burst_test" \
    "--url $URL --readers 200 --duration 30s --rate 50/s" \
    "35s"

# 6. Sustained load test
run_test "sustained_load" \
    "--url $URL --readers 50 --duration 5m --rate 2/s" \
    "305s"

# Summary
echo "════════════════════════════════════════════════════════════════"
echo "All tests completed!"
echo "Results available in: $LOG_DIR"
echo "════════════════════════════════════════════════════════════════"