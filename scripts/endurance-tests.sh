#!/bin/bash
# Created by WINK Streaming (https://www.wink.co)
# Long-duration endurance tests

BENCH="/opt/wink-rtsp-bench/wink-rtsp-bench"
URL="rtsp://localhost:8554/bunny"
LOG_DIR="/opt/wink-rtsp-bench/test-results/endurance"

mkdir -p "$LOG_DIR"

# Function to run endurance test
run_endurance() {
    local name=$1
    local hours=$2
    local readers=$3
    local mode=$4
    
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║     Starting Endurance Test: $name"
    echo "║     Duration: $hours hours"
    echo "║     Mode: $mode"
    echo "╚══════════════════════════════════════════════════════════════╝"
    
    log_file="$LOG_DIR/${name}_${hours}h_$(date +%Y%m%d_%H%M%S).log"
    
    if [ "$mode" = "realworld" ]; then
        nohup $BENCH \
            --url "$URL" \
            --real-world \
            --avg-connections "$readers" \
            --variance 0.4 \
            --hours "$hours" \
            --rate 10/s \
            --stats-interval 30s \
            --log json > "$log_file" 2>&1 &
    else
        nohup $BENCH \
            --url "$URL" \
            --readers "$readers" \
            --hours "$hours" \
            --rate 5/s \
            --stats-interval 30s \
            --log json > "$log_file" 2>&1 &
    fi
    
    PID=$!
    echo "Test started with PID: $PID"
    echo "Log file: $log_file"
    echo "PID: $PID" > "$LOG_DIR/${name}_${hours}h.pid"
    echo ""
}

# Check for command
case "$1" in
    start)
        case "$2" in
            1h)
                run_endurance "endurance_1h" 1 100 "fixed"
                ;;
            24h)
                run_endurance "endurance_24h" 24 500 "realworld"
                ;;
            48h)
                run_endurance "endurance_48h" 48 500 "realworld"
                ;;
            72h)
                run_endurance "endurance_72h" 72 500 "realworld"
                ;;
            *)
                echo "Usage: $0 start {1h|24h|48h|72h}"
                exit 1
                ;;
        esac
        ;;
    stop)
        echo "Stopping all endurance tests..."
        for pidfile in $LOG_DIR/*.pid; do
            if [ -f "$pidfile" ]; then
                PID=$(cat "$pidfile")
                if kill -0 "$PID" 2>/dev/null; then
                    echo "Stopping PID: $PID"
                    kill "$PID"
                    rm "$pidfile"
                fi
            fi
        done
        ;;
    status)
        echo "Active endurance tests:"
        for pidfile in $LOG_DIR/*.pid; do
            if [ -f "$pidfile" ]; then
                PID=$(cat "$pidfile")
                if kill -0 "$PID" 2>/dev/null; then
                    echo "  - $(basename $pidfile .pid) (PID: $PID) - RUNNING"
                else
                    echo "  - $(basename $pidfile .pid) - STOPPED"
                    rm "$pidfile"
                fi
            fi
        done
        ;;
    *)
        echo "Usage: $0 {start|stop|status} [duration]"
        echo "Durations: 1h, 24h, 48h, 72h"
        exit 1
        ;;
esac