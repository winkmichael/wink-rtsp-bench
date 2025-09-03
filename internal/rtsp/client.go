// Created by WINK Streaming (https://www.wink.co)
package rtsp

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/winkstreaming/wink-rtsp-bench/internal/rtp"
)

const (
	DefaultRTSPPort = 554
	KeepAliveInterval = 20 * time.Second
	ReadTimeout = 10 * time.Second
)

// Client represents an RTSP client connection
type Client struct {
	url        *url.URL
	transport  string
	conn       net.Conn
	reader     *bufio.Reader
	session    string
	cseq       int
	aggregator *rtp.Aggregator
	tracker    *rtp.SeqTracker
	
	// UDP specific
	rtpConn    net.PacketConn
	rtcpConn   net.PacketConn
	serverRTP  int
	serverRTCP int
	
	mu         sync.Mutex
	closed     bool
	
	// Stats
	bytesReceived uint64
	packetsRcvd   uint64
}

// NewClient creates a new RTSP client
func NewClient(rtspURL string, transport string, agg *rtp.Aggregator) (*Client, error) {
	u, err := url.Parse(rtspURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	if transport == "" {
		transport = "tcp"
	}

	return &Client{
		url:        u,
		transport:  strings.ToLower(transport),
		cseq:       1,
		aggregator: agg,
		tracker:    rtp.NewSeqTracker(),
	}, nil
}

// Connect establishes the RTSP control connection
func (c *Client) Connect() error {
	host := c.url.Host
	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:%d", host, DefaultRTSPPort)
	}

	conn, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	c.conn = conn
	// Use much larger buffer to prevent overflow on long RTSP responses
	// MediaMTX can send very large SDP bodies  
	c.reader = bufio.NewReaderSize(conn, 1024*1024) // 1MB buffer
	return nil
}

// Run executes the RTSP session until context is cancelled
func (c *Client) Run(ctx context.Context) error {
	// Connect if not already connected
	if c.conn == nil {
		if err := c.Connect(); err != nil {
			return err
		}
	}
	defer c.Close()

	// RTSP handshake: OPTIONS -> DESCRIBE -> SETUP -> PLAY
	if err := c.sendOptions(); err != nil {
		return fmt.Errorf("OPTIONS failed: %w", err)
	}

	if err := c.sendDescribe(); err != nil {
		return fmt.Errorf("DESCRIBE failed: %w", err)
	}

	if err := c.sendSetup(); err != nil {
		return fmt.Errorf("SETUP failed: %w", err)
	}

	if err := c.sendPlay(); err != nil {
		return fmt.Errorf("PLAY failed: %w", err)
	}

	// Start media reception based on transport
	if c.transport == "udp" {
		return c.runUDP(ctx)
	}
	return c.runTCP(ctx)
}

// runTCP handles TCP interleaved RTP reception
func (c *Client) runTCP(ctx context.Context) error {
	keepAlive := time.NewTicker(KeepAliveInterval)
	defer keepAlive.Stop()

	// Channel for keepalive errors
	errCh := make(chan error, 1)

	for {
		select {
		case <-ctx.Done():
			c.reportStats()
			return ctx.Err()
		case <-keepAlive.C:
			go func() {
				if err := c.sendKeepAlive(); err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			}()
		case err := <-errCh:
			return fmt.Errorf("keepalive failed: %w", err)
		default:
			// Read interleaved frame
			if err := c.readInterleavedFrame(); err != nil {
				if ctx.Err() != nil {
					c.reportStats()
					return nil
				}
				return fmt.Errorf("read frame failed: %w", err)
			}
		}
	}
}

// runUDP handles UDP RTP reception
func (c *Client) runUDP(ctx context.Context) error {
	// Set up UDP listeners if not already done
	if c.rtpConn == nil {
		rtpConn, err := net.ListenPacket("udp", ":0")
		if err != nil {
			return fmt.Errorf("failed to create RTP socket: %w", err)
		}
		// Increase receive buffer size for better performance
		if conn, ok := rtpConn.(*net.UDPConn); ok {
			conn.SetReadBuffer(2 * 1024 * 1024) // 2MB buffer
		}
		c.rtpConn = rtpConn
		defer rtpConn.Close()

		rtcpConn, err := net.ListenPacket("udp", ":0")
		if err != nil {
			return fmt.Errorf("failed to create RTCP socket: %w", err)
		}
		c.rtcpConn = rtcpConn
		defer rtcpConn.Close()
	}

	// Start keepalive goroutine
	keepAliveCtx, cancelKeepAlive := context.WithCancel(ctx)
	defer cancelKeepAlive()
	
	keepAliveErr := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(KeepAliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-keepAliveCtx.Done():
				return
			case <-ticker.C:
				if err := c.sendKeepAlive(); err != nil {
					select {
					case keepAliveErr <- err:
					default:
					}
					return
				}
			}
		}
	}()

	// Use larger buffer for UDP packets
	buf := make([]byte, 65536) // 64KB buffer for jumbo frames
	
	// Set a longer deadline to reduce syscall overhead
	c.rtpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	deadlineTimer := time.NewTicker(10 * time.Second)
	defer deadlineTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			c.reportStats()
			return ctx.Err()
		case err := <-keepAliveErr:
			return fmt.Errorf("keepalive failed: %w", err)
		case <-deadlineTimer.C:
			// Refresh deadline periodically
			c.rtpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		default:
			n, _, err := c.rtpConn.ReadFrom(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					// Refresh deadline on timeout
					c.rtpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
					continue
				}
				if ctx.Err() != nil {
					c.reportStats()
					return nil
				}
				return fmt.Errorf("UDP read failed: %w", err)
			}

			// Process RTP packet
			if n >= 12 {
				// Make a copy to avoid data races
				packet := make([]byte, n)
				copy(packet, buf[:n])
				c.processRTPPacket(packet)
			}
		}
	}
}

// readInterleavedFrame reads a TCP interleaved RTP/RTCP frame
func (c *Client) readInterleavedFrame() error {
	// Read magic byte
	magic, err := c.reader.ReadByte()
	if err != nil {
		return err
	}

	// Check for RTSP response (not interleaved data)
	if magic != '$' {
		// Might be RTSP response, consume the line
		// Use a loop to handle very long lines that might exceed buffer
		var line string
		for {
			partial, err := c.reader.ReadString('\n')
			if err != nil && err != bufio.ErrBufferFull {
				return err
			}
			line += partial
			if err != bufio.ErrBufferFull {
				break
			}
		}
		_ = line // Ignore unsolicited responses
		return nil
	}

	// Read channel
	channel, err := c.reader.ReadByte()
	if err != nil {
		return err
	}

	// Read length (16-bit big endian)
	var length uint16
	if err := binary.Read(c.reader, binary.BigEndian, &length); err != nil {
		return err
	}

	// Read payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return err
	}

	// Process based on channel (0=RTP, 1=RTCP typically)
	if channel == 0 && len(payload) >= 12 {
		c.processRTPPacket(payload)
	}

	c.bytesReceived += uint64(4 + length)
	return nil
}

// processRTPPacket extracts sequence number and updates tracking
func (c *Client) processRTPPacket(data []byte) {
	if len(data) < 12 {
		return
	}

	// Extract sequence number (bytes 2-3)
	seq := binary.BigEndian.Uint16(data[2:4])
	
	// Track sequence
	lost := c.tracker.Push(seq)
	c.packetsRcvd++

	// Update aggregator
	if lost > 0 {
		c.aggregator.AddLoss(lost)
	}
	c.aggregator.AddPackets(1)

	c.bytesReceived += uint64(len(data))
}

// sendOptions sends RTSP OPTIONS request
func (c *Client) sendOptions() error {
	req := c.buildRequest("OPTIONS", nil)
	return c.sendRequest(req)
}

// sendDescribe sends RTSP DESCRIBE request
func (c *Client) sendDescribe() error {
	headers := map[string]string{
		"Accept": "application/sdp",
	}
	req := c.buildRequest("DESCRIBE", headers)
	return c.sendRequest(req)
}

// sendSetup sends RTSP SETUP request for each track
func (c *Client) sendSetup() error {
	// First, we need to know about tracks - for now assume standard video/audio
	// In production, parse SDP from DESCRIBE response
	
	// Setup video track (trackID=0)
	headers := make(map[string]string)
	if c.transport == "udp" {
		// For UDP, allocate local ports for video track
		if c.rtpConn == nil {
			rtpConn, err := net.ListenPacket("udp", ":0")
			if err != nil {
				return err
			}
			c.rtpConn = rtpConn

			rtcpConn, err := net.ListenPacket("udp", ":0")
			if err != nil {
				return err
			}
			c.rtcpConn = rtcpConn
		}

		rtpPort := c.rtpConn.LocalAddr().(*net.UDPAddr).Port
		rtcpPort := c.rtcpConn.LocalAddr().(*net.UDPAddr).Port
		
		headers["Transport"] = fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d", rtpPort, rtcpPort)
	} else {
		// TCP interleaved for video
		headers["Transport"] = "RTP/AVP/TCP;unicast;interleaved=0-1"
	}

	// Setup video track
	req := c.buildTrackRequest("SETUP", "/trackID=0", headers)
	resp, err := c.sendRequestWithResponse(req)
	if err != nil {
		return err
	}

	// Extract session ID from first SETUP response
	if session := c.extractHeader(resp, "Session"); session != "" {
		parts := strings.Split(session, ";")
		c.session = strings.TrimSpace(parts[0])
	}

	// For UDP, we could extract and store server ports from video track response
	// but MediaMTX has specific UDP handling that makes this complex
	// UDP support is best-effort for now

	// Setup audio track (trackID=1) - using same session but different ports for UDP
	if c.session != "" {
		headers = make(map[string]string)
		headers["Session"] = c.session
		if c.transport == "tcp" {
			headers["Transport"] = "RTP/AVP/TCP;unicast;interleaved=2-3"
		} else if c.transport == "udp" {
			// For UDP audio, we'll use the same sockets but different server ports
			// Just reuse the same client ports for simplicity
			rtpPort := c.rtpConn.LocalAddr().(*net.UDPAddr).Port
			rtcpPort := c.rtcpConn.LocalAddr().(*net.UDPAddr).Port
			headers["Transport"] = fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d", rtpPort, rtcpPort)
		}
		
		req = c.buildTrackRequest("SETUP", "/trackID=1", headers)
		_, err = c.sendRequestWithResponse(req)
		// Ignore audio track errors - video only is OK
	}

	// For UDP, store server address for sending RTCP reports (not implemented yet)
	// In a full implementation, we'd connect our UDP sockets to the server ports here

	return nil
}

// sendPlay sends RTSP PLAY request
func (c *Client) sendPlay() error {
	headers := map[string]string{
		"Session": c.session,
		"Range":   "npt=0.000-",
	}
	req := c.buildRequest("PLAY", headers)
	return c.sendRequest(req)
}

// sendKeepAlive sends a keep-alive request (GET_PARAMETER or OPTIONS)
func (c *Client) sendKeepAlive() error {
	headers := map[string]string{
		"Session": c.session,
	}
	req := c.buildRequest("GET_PARAMETER", headers)
	return c.sendRequest(req)
}

// sendTeardown sends RTSP TEARDOWN request
func (c *Client) sendTeardown() error {
	if c.session == "" {
		return nil
	}
	
	headers := map[string]string{
		"Session": c.session,
	}
	req := c.buildRequest("TEARDOWN", headers)
	return c.sendRequest(req)
}

// buildRequest constructs an RTSP request
func (c *Client) buildRequest(method string, headers map[string]string) string {
	var b strings.Builder
	
	// Request line
	uri := fmt.Sprintf("%s://%s%s", c.url.Scheme, c.url.Host, c.url.Path)
	b.WriteString(fmt.Sprintf("%s %s RTSP/1.0\r\n", method, uri))
	
	// CSeq header
	b.WriteString(fmt.Sprintf("CSeq: %d\r\n", c.cseq))
	c.cseq++
	
	// User-Agent
	b.WriteString("User-Agent: WINK-RTSP-Bench/1.0\r\n")
	
	// Additional headers
	for key, value := range headers {
		b.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	
	// End of headers
	b.WriteString("\r\n")
	
	return b.String()
}

// buildTrackRequest constructs an RTSP request for a specific track
func (c *Client) buildTrackRequest(method string, trackPath string, headers map[string]string) string {
	var b strings.Builder
	
	// Request line with track path appended
	uri := fmt.Sprintf("%s://%s%s%s", c.url.Scheme, c.url.Host, c.url.Path, trackPath)
	b.WriteString(fmt.Sprintf("%s %s RTSP/1.0\r\n", method, uri))
	
	// CSeq header
	b.WriteString(fmt.Sprintf("CSeq: %d\r\n", c.cseq))
	c.cseq++
	
	// User-Agent
	b.WriteString("User-Agent: WINK-RTSP-Bench/1.0\r\n")
	
	// Additional headers
	for key, value := range headers {
		b.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	
	// End of headers
	b.WriteString("\r\n")
	
	return b.String()
}

// sendRequest sends a request and reads response (discarding body)
func (c *Client) sendRequest(req string) error {
	_, err := c.sendRequestWithResponse(req)
	return err
}

// sendRequestWithResponse sends request and returns full response
func (c *Client) sendRequestWithResponse(req string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return "", fmt.Errorf("connection closed")
	}

	// Send request
	if _, err := c.conn.Write([]byte(req)); err != nil {
		return "", err
	}

	// Read response
	return c.readResponse()
}

// readResponse reads an RTSP response
func (c *Client) readResponse() (string, error) {
	var response strings.Builder
	
	// Read status line with proper handling for long lines
	var statusLine string
	for {
		partial, err := c.reader.ReadString('\n')
		if err != nil && err != bufio.ErrBufferFull {
			return "", err
		}
		statusLine += partial
		if err != bufio.ErrBufferFull {
			break
		}
	}
	response.WriteString(statusLine)
	
	// Check status code
	if !strings.HasPrefix(statusLine, "RTSP/1.0") {
		return "", fmt.Errorf("invalid response: %s", statusLine)
	}
	
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return "", fmt.Errorf("malformed status line")
	}
	
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid status code: %s", parts[1])
	}
	
	// Read headers
	contentLength := 0
	for {
		// Read header line with proper buffer handling
		var line string
		for {
			partial, err := c.reader.ReadString('\n')
			if err != nil && err != bufio.ErrBufferFull {
				return "", err
			}
			line += partial
			if err != bufio.ErrBufferFull {
				break
			}
		}
		response.WriteString(line)
		
		// End of headers
		if line == "\r\n" || line == "\n" {
			break
		}
		
		// Parse Content-Length if present
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				contentLength, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}
	
	// Read body if present
	if contentLength > 0 {
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(c.reader, body); err != nil {
			return "", err
		}
		response.Write(body)
	}
	
	// Check for error status
	if statusCode >= 400 {
		return response.String(), fmt.Errorf("RTSP error %d", statusCode)
	}
	
	return response.String(), nil
}

// extractHeader extracts a header value from response
func (c *Client) extractHeader(response, header string) string {
	lines := strings.Split(response, "\n")
	header = strings.ToLower(header)
	
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), header+":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// parseTransportHeader extracts server ports from Transport header
func (c *Client) parseTransportHeader(transport string) {
	// Example: RTP/AVP;unicast;client_port=5000-5001;server_port=6000-6001
	parts := strings.Split(transport, ";")
	for _, part := range parts {
		if strings.HasPrefix(part, "server_port=") {
			ports := strings.TrimPrefix(part, "server_port=")
			portParts := strings.Split(ports, "-")
			if len(portParts) >= 1 {
				c.serverRTP, _ = strconv.Atoi(portParts[0])
				if len(portParts) >= 2 {
					c.serverRTCP, _ = strconv.Atoi(portParts[1])
				}
			}
		}
	}
}

// reportStats reports final statistics to aggregator
func (c *Client) reportStats() {
	if c.tracker != nil {
		stats := c.tracker.GetStats()
		if stats.Lost > 0 {
			c.aggregator.AddLoss(stats.Lost)
		}
	}
}

// Close closes the RTSP connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Send TEARDOWN if we have a session
	if c.session != "" && c.conn != nil {
		c.sendTeardown()
	}

	// Close connections
	if c.conn != nil {
		c.conn.Close()
	}
	if c.rtpConn != nil {
		c.rtpConn.Close()
	}
	if c.rtcpConn != nil {
		c.rtcpConn.Close()
	}

	return nil
}