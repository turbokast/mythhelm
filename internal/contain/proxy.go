package contain

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

// proxyHeaderLimit caps how many header lines one client request may carry;
// a CONNECT request that exceeds it is rejected without being forwarded.
const proxyHeaderLimit = 64

// ServeProxy starts a worker-owned localhost listener that forwards CONNECT
// tunnels only to an allowlist of exact "host:port" entries. Anything else
// gets an explicit denial status (403 unlisted, 405 non-CONNECT) and no bytes
// leave the proxy. Direct egress around the proxy is NOT blocked — that is
// the spec's disclosed residual, never a claim this proxy makes. It returns
// the proxy's own "host:port" address and a stop func that is safe to call
// twice. The proxy also stops when ctx is done.
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
	var once sync.Once
	stop = func() { once.Do(func() { _ = ln.Close() }) }
	go func() {
		<-ctx.Done()
		stop()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // Listener closed by stop or context cancellation.
			}
			go serveProxyConn(conn, allowed)
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

// serveProxyConn answers one client connection: exactly one CONNECT to an
// allowlisted host:port is tunnelled; everything else is denied and the
// connection closed without contacting the target.
func serveProxyConn(conn net.Conn, allowed map[string]struct{}) {
	defer func() { _ = conn.Close() }()
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		proxyWriteStatus(conn, "400 Bad Request")
		return
	}
	method, target := fields[0], strings.TrimSpace(fields[1])
	// Drain the request headers (bounded) so a denied or tunnelled request
	// leaves no unread bytes behind on this connection.
	for i := range proxyHeaderLimit {
		hdr, err := br.ReadString('\n')
		if err != nil {
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
	if method != "CONNECT" {
		proxyWriteStatus(conn, "405 Method Not Allowed")
		return
	}
	if _, ok := allowed[target]; !ok {
		proxyWriteStatus(conn, "403 Forbidden")
		return
	}
	upstream, err := net.Dial("tcp", target)
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
