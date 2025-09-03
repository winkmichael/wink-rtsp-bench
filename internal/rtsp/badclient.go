// Created by WINK Streaming (https://www.wink.co)
package rtsp

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

// BadClientType represents different types of misbehaving clients
type BadClientType int

const (
	SlowConnector BadClientType = iota  // Connects very slowly
	SlowSender                          // Sends messages extremely slowly
	GarbageSender                       // Sends random garbage data
	IncompleteHandshake                 // Starts handshake but never completes
	InvalidProtocol                     // Sends invalid RTSP commands
	ResourceHog                         // Connects and holds resources without activity
	RandomDisconnect                    // Disconnects at random times
	MalformedRequests                   // Sends malformed RTSP requests
)

// BadClient represents a misbehaving RTSP client for stress testing
type BadClient struct {
	url       string
	clientType BadClientType
	conn      net.Conn
}

// NewBadClient creates a new misbehaving client
func NewBadClient(url string) *BadClient {
	// Randomly select a bad behavior type
	clientType := BadClientType(rand.Intn(8))
	
	return &BadClient{
		url:        url,
		clientType: clientType,
	}
}

// Run executes the bad client behavior
func (bc *BadClient) Run(ctx context.Context) error {
	switch bc.clientType {
	case SlowConnector:
		return bc.runSlowConnector(ctx)
	case SlowSender:
		return bc.runSlowSender(ctx)
	case GarbageSender:
		return bc.runGarbageSender(ctx)
	case IncompleteHandshake:
		return bc.runIncompleteHandshake(ctx)
	case InvalidProtocol:
		return bc.runInvalidProtocol(ctx)
	case ResourceHog:
		return bc.runResourceHog(ctx)
	case RandomDisconnect:
		return bc.runRandomDisconnect(ctx)
	case MalformedRequests:
		return bc.runMalformedRequests(ctx)
	default:
		return bc.runGarbageSender(ctx)
	}
}

// runSlowConnector connects extremely slowly
func (bc *BadClient) runSlowConnector(ctx context.Context) error {
	// Parse URL to get host
	parts := strings.Split(bc.url, "://")
	if len(parts) < 2 {
		return fmt.Errorf("invalid URL")
	}
	
	hostParts := strings.Split(parts[1], "/")
	host := hostParts[0]
	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:8554", host)
	}
	
	// Start connection but do it very slowly
	conn, err := net.DialTimeout("tcp", host, 30*time.Second)
	if err != nil {
		return err
	}
	bc.conn = conn
	defer conn.Close()
	
	// Send OPTIONS very slowly (1 byte per second)
	message := "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	for i, ch := range []byte(message) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(100+rand.Intn(900)) * time.Millisecond):
			if _, err := conn.Write([]byte{ch}); err != nil {
				return err
			}
			// Occasionally pause for longer
			if i%10 == 0 {
				time.Sleep(time.Duration(1+rand.Intn(3)) * time.Second)
			}
		}
	}
	
	// Keep connection open until context cancels
	<-ctx.Done()
	return nil
}

// runSlowSender sends valid RTSP but extremely slowly
func (bc *BadClient) runSlowSender(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	cseq := 1
	commands := []string{"OPTIONS * RTSP/1.0", "DESCRIBE %s RTSP/1.0"}
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			cmd := fmt.Sprintf(commands[cseq%len(commands)], bc.url)
			if cseq%len(commands) == 1 {
				cmd = fmt.Sprintf(cmd, bc.url)
			}
			message := fmt.Sprintf("%s\r\nCSeq: %d\r\n\r\n", cmd, cseq)
			
			// Send each character with random delays
			for _, ch := range []byte(message) {
				delay := time.Duration(50+rand.Intn(450)) * time.Millisecond
				time.Sleep(delay)
				if _, err := bc.conn.Write([]byte{ch}); err != nil {
					return err
				}
			}
			
			cseq++
			// Long pause between commands
			time.Sleep(time.Duration(5+rand.Intn(10)) * time.Second)
		}
	}
}

// runGarbageSender sends random garbage data
func (bc *BadClient) runGarbageSender(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	garbage := []string{
		"GET / HTTP/1.1\r\n\r\n",  // Wrong protocol
		"HELLO RTSP SERVER\n",
		"\x00\x01\x02\x03\x04\x05\x06\x07",  // Binary garbage
		"OPTIONS * RTSP/2.0\r\n\r\n",  // Wrong version
		"<?xml version=\"1.0\"?><root></root>",  // XML garbage
		"CONNECT proxy.example.com:443 HTTP/1.1\r\n\r\n",
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit...",
		string(make([]byte, 1000)),  // Null bytes
	}
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Send random garbage
			data := garbage[rand.Intn(len(garbage))]
			if rand.Float32() < 0.3 {
				// Sometimes send completely random bytes
				randomBytes := make([]byte, 100+rand.Intn(900))
				_, _ = rand.Read(randomBytes) // crypto/rand.Read rarely fails
				data = string(randomBytes)
			}
			
			if _, err := bc.conn.Write([]byte(data)); err != nil {
				return err
			}
			
			// Random delay
			time.Sleep(time.Duration(100+rand.Intn(2000)) * time.Millisecond)
		}
	}
}

// runIncompleteHandshake starts RTSP handshake but never completes it
func (bc *BadClient) runIncompleteHandshake(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	// Send OPTIONS
	options := "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := bc.conn.Write([]byte(options)); err != nil {
		return err
	}
	
	// Read response but ignore it (errors expected for bad clients)
	buf := make([]byte, 1024)
	_ = bc.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _ = bc.conn.Read(buf)
	
	// Send DESCRIBE but incomplete
	describe := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 2\r\n", bc.url)
	if _, err := bc.conn.Write([]byte(describe)); err != nil {
		return err
	}
	// Never send the final \r\n
	
	// Just hold the connection open
	<-ctx.Done()
	return nil
}

// runInvalidProtocol sends syntactically incorrect RTSP
func (bc *BadClient) runInvalidProtocol(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	invalidCommands := []string{
		"OPTIONS\r\n\r\n",  // Missing version
		"RTSP/1.0 OPTIONS *\r\n\r\n",  // Wrong order
		"OPTIONS * RTSP/1.0\r\nCSeq\r\n\r\n",  // Incomplete header
		"OPTIONS * RTSP/1.0\r\nCSeq: -1\r\n\r\n",  // Invalid CSeq
		"PLAY RTSP/1.0\r\n\r\n",  // Missing URL
		"OPTIONS * RTSP/1.0\nCSeq: 1\n\n",  // Wrong line endings
		"OPTIONS * RTSP/1.0\r\nCSeq: 1\r\nContent-Length: 100\r\n\r\n",  // Wrong content length
		"HACK * RTSP/1.0\r\nCSeq: 1\r\n\r\n",  // Invalid method
	}
	
	cseq := 1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			cmd := invalidCommands[rand.Intn(len(invalidCommands))]
			// Sometimes inject the current CSeq
			if strings.Contains(cmd, "CSeq: 1") {
				cmd = strings.Replace(cmd, "CSeq: 1", fmt.Sprintf("CSeq: %d", cseq), 1)
			}
			
			if _, err := bc.conn.Write([]byte(cmd)); err != nil {
				return err
			}
			
			cseq++
			time.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond)
		}
	}
}

// runResourceHog connects and holds resources without proper activity
func (bc *BadClient) runResourceHog(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	// Send initial OPTIONS to establish connection
	options := "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := bc.conn.Write([]byte(options)); err != nil {
		return err
	}
	
	// Read and discard response (errors expected for bad clients)
	buf := make([]byte, 4096)
	_ = bc.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _ = bc.conn.Read(buf)
	
	// Now just hold the connection open, occasionally sending incomplete data
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Send a single byte to keep connection alive but not complete any command
			_, _ = bc.conn.Write([]byte("O")) // Ignore errors - connection may be closing
		}
	}
}

// runRandomDisconnect connects properly then disconnects at random times
func (bc *BadClient) runRandomDisconnect(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	// Random duration before disconnect (between 1 and 30 seconds)
	duration := time.Duration(1+rand.Intn(30)) * time.Second
	
	// Send OPTIONS
	options := "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := bc.conn.Write([]byte(options)); err != nil {
		return err
	}
	
	// Wait then abruptly close
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		// Abrupt close without proper teardown
		bc.conn.Close()
		return fmt.Errorf("intentional random disconnect")
	}
}

// runMalformedRequests sends requests with various malformations
func (bc *BadClient) runMalformedRequests(ctx context.Context) error {
	if err := bc.connect(); err != nil {
		return err
	}
	defer bc.conn.Close()
	
	cseq := 1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Generate malformed request
			var request string
			switch rand.Intn(6) {
			case 0:
				// Huge header value
				request = fmt.Sprintf("OPTIONS * RTSP/1.0\r\nCSeq: %d\r\nUser-Agent: %s\r\n\r\n",
					cseq, strings.Repeat("A", 10000))
			case 1:
				// Many headers
				var headers strings.Builder
				headers.WriteString(fmt.Sprintf("OPTIONS * RTSP/1.0\r\nCSeq: %d\r\n", cseq))
				for i := 0; i < 1000; i++ {
					headers.WriteString(fmt.Sprintf("X-Header-%d: value\r\n", i))
				}
				headers.WriteString("\r\n")
				request = headers.String()
			case 2:
				// Unicode in headers
				request = fmt.Sprintf("OPTIONS * RTSP/1.0\r\nCSeq: %d\r\nX-Test: 你好世界\r\n\r\n", cseq)
			case 3:
				// Null bytes in request
				request = fmt.Sprintf("OPTIONS * RTSP/1.0\r\nCSeq: %d\r\nX-Null: \x00\x00\x00\r\n\r\n", cseq)
			case 4:
				// Very long URL
				request = fmt.Sprintf("DESCRIBE rtsp://example.com/%s RTSP/1.0\r\nCSeq: %d\r\n\r\n",
					strings.Repeat("path/", 1000), cseq)
			case 5:
				// Mixed case methods
				methods := []string{"OpTiOnS", "options", "OPTIONS", "oPtIoNs"}
				request = fmt.Sprintf("%s * RTSP/1.0\r\nCSeq: %d\r\n\r\n",
					methods[rand.Intn(len(methods))], cseq)
			}
			
			if _, err := bc.conn.Write([]byte(request)); err != nil {
				return err
			}
			
			// Try to read response but don't care about it
			buf := make([]byte, 4096)
			_ = bc.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, _ = bc.conn.Read(buf)
			
			cseq++
			time.Sleep(time.Duration(200+rand.Intn(800)) * time.Millisecond)
		}
	}
}

// connect establishes a basic TCP connection
func (bc *BadClient) connect() error {
	// Parse URL to get host
	parts := strings.Split(bc.url, "://")
	if len(parts) < 2 {
		return fmt.Errorf("invalid URL")
	}
	
	hostParts := strings.Split(parts[1], "/")
	host := hostParts[0]
	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:8554", host)
	}
	
	conn, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return err
	}
	
	bc.conn = conn
	return nil
}

// GetTypeName returns a human-readable name for the bad client type
func (bc *BadClient) GetTypeName() string {
	names := []string{
		"SlowConnector",
		"SlowSender",
		"GarbageSender",
		"IncompleteHandshake",
		"InvalidProtocol",
		"ResourceHog",
		"RandomDisconnect",
		"MalformedRequests",
	}
	
	if int(bc.clientType) < len(names) {
		return names[bc.clientType]
	}
	return "Unknown"
}