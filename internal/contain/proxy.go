package contain

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// proxyHeaderLimit caps how many header lines one client request may carry;
// a CONNECT request that exceeds it is rejected without being forwarded.
const proxyHeaderLimit = 64

// proxyLineLimit caps one request or header line; longer lines are rejected
// with 431 before they can retain unbounded memory.
const proxyLineLimit = 8192

// proxyRequestTimeout bounds the request phase of one connection so a client
// that connects and sends nothing cannot hold a slot forever.
const proxyRequestTimeout = 10 * time.Second

// proxyDialTimeout bounds establishing the upstream side of a tunnel.
const proxyDialTimeout = 10 * time.Second

// proxyMaxConns caps concurrent client connections; excess connections are
// closed instead of spawning unbounded goroutines.
const proxyMaxConns = 32

// proxyDrainLimit caps how much unread request input a 431 denial drains
// before closing, so the client receives the response instead of a reset.
const proxyDrainLimit = 1 << 20

// ServeProxy starts a worker-owned localhost listener that forwards CONNECT
// tunnels only to an allowlist of exact "host:port" entries. Anything else
// gets an explicit denial status (403 unlisted, 405 non-CONNECT) and no bytes
// leave the proxy. Direct egress around the proxy is NOT blocked — that is
// the spec's disclosed residual, never a claim this proxy makes. It returns
// the proxy's own "host:port" address and a stop func that is safe to call
// twice. Stopping closes the listener and every active connection, so no
// tunnel relays past a stop. The proxy also stops when ctx is done.
func ServeProxy(ctx context.Context, allow []string) (addr string, stop func(), err error) {
	allowed := make(map[string]struct{}, len(allow))
	for _, entry := range allow {
		allowed[strings.TrimSpace(entry)] = struct{}{}
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("contain: listen proxy: %w", err)
	}
	var mu sync.Mutex
	active := make(map[net.Conn]struct{})
	var once sync.Once
	stop = func() {
		once.Do(func() {
			_ = ln.Close()
			mu.Lock()
			defer mu.Unlock()
			for conn := range active {
				_ = conn.Close()
			}
		})
	}
	go func() {
		<-ctx.Done()
		stop()
	}()
	sem := make(chan struct{}, proxyMaxConns)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // Listener closed by stop or context cancellation.
			}
			select {
			case sem <- struct{}{}:
				mu.Lock()
				active[conn] = struct{}{}
				mu.Unlock()
				go func() {
					defer func() {
						mu.Lock()
						delete(active, conn)
						mu.Unlock()
						<-sem
					}()
					serveProxyConn(conn, allowed)
				}()
			default:
				proxyWriteStatus(conn, "503 Service Unavailable")
				_ = conn.Close()
			}
		}
	}()
	return ln.Addr().String(), stop, nil
}

// ProxyEnv pins a contained process's proxy variables at the address a
// ServeProxy call returned.
func ProxyEnv(addr string) map[string]string {
	url := "http://" + addr
	return map[string]string{
		"HTTPS_PROXY": url,
		"HTTP_PROXY":  url,
		"https_proxy": url,
		"http_proxy":  url,
	}
}

// proxyReadLine reads one '\n'-terminated line of at most proxyLineLimit
// bytes; a longer line reports tooLarge instead of retaining more memory.
func proxyReadLine(br *bufio.Reader) (line string, tooLarge bool, err error) {
	raw, err := br.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(raw), false, nil
}

// proxyDenyTooLarge answers an oversized request or header line with 431,
// then half-closes and drains bounded unread input (under the request-phase
// read deadline) so the client receives the response instead of a TCP reset
// triggered by closing with unread received data.
func proxyDenyTooLarge(conn net.Conn, br *bufio.Reader) {
	proxyWriteStatus(conn, "431 Request Header Fields Too Large")
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	_, _ = io.CopyN(io.Discard, br, proxyDrainLimit)
}

// serveProxyConn answers one client connection: exactly one CONNECT to an
// allowlisted host:port is tunnelled; everything else is denied and the
// connection closed without contacting the target.
func serveProxyConn(conn net.Conn, allowed map[string]struct{}) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(proxyRequestTimeout))
	br := bufio.NewReaderSize(conn, proxyLineLimit)
	line, tooLarge, err := proxyReadLine(br)
	if err != nil {
		return
	}
	if tooLarge {
		proxyDenyTooLarge(conn, br)
		return
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		proxyWriteStatus(conn, "400 Bad Request")
		return
	}
	method, target := fields[0], strings.TrimSpace(fields[1])
	// Drain the request headers (bounded in count and length) so a denied
	// or tunnelled request leaves no unread bytes behind on this connection.
	for i := range proxyHeaderLimit {
		hdr, hdrLarge, err := proxyReadLine(br)
		if err != nil {
			return
		}
		if hdrLarge {
			proxyDenyTooLarge(conn, br)
			return
		}
		if hdr == "\r\n" || hdr == "\n" {
			break
		}
		if i == proxyHeaderLimit-1 {
			proxyWriteStatus(conn, "431 Request Header Fields Too Large")
			return
		}
	}
	_ = conn.SetReadDeadline(time.Time{}) // Request phase done; tunnel streams freely.
	if method != "CONNECT" {
		proxyWriteStatus(conn, "405 Method Not Allowed")
		return
	}
	if _, ok := allowed[target]; !ok {
		proxyWriteStatus(conn, "403 Forbidden")
		return
	}
	upstream, err := (&net.Dialer{Timeout: proxyDialTimeout}).Dial("tcp", target)
	if err != nil {
		proxyWriteStatus(conn, "502 Bad Gateway")
		return
	}
	defer func() { _ = upstream.Close() }()
	proxyWriteStatus(conn, "200 Connection Established")
	proxyTunnel(conn, br, upstream)
}

// proxyTunnel copies bytes both ways until either side ends the connection,
// then closes both ends so the other copy cannot block forever.
func proxyTunnel(client net.Conn, buffered *bufio.Reader, upstream net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, buffered)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
	_ = client.Close()
	_ = upstream.Close()
	<-done
}

func proxyWriteStatus(w io.Writer, status string) {
	_, _ = fmt.Fprintf(w, "HTTP/1.1 %s\r\nConnection: close\r\nContent-Length: 0\r\n\r\n", status)
}
