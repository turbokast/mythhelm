package contain

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// proxyHostPort strips the http:// scheme from an httptest server URL,
// leaving the exact host:port a CONNECT request-target must name.
func proxyHostPort(t *testing.T, url string) string {
	t.Helper()
	hostport, ok := strings.CutPrefix(url, "http://")
	if !ok {
		t.Fatalf("test server URL %q is not plain http", url)
	}
	return hostport
}

// proxyWritef writes one request to conn, failing the test on a short write.
// A broken test connection fails fast here instead of surfacing as a
// confusing read error later.
func proxyWritef(t *testing.T, conn net.Conn, format string, args ...any) {
	t.Helper()
	if _, err := fmt.Fprintf(conn, format, args...); err != nil {
		_ = conn.Close()
		t.Fatalf("write request: %v", err)
	}
}

// proxyConnect dials the proxy at addr, sends one CONNECT for target, and
// returns the status line plus a reader positioned at the tunnel bytes.
func proxyConnect(t *testing.T, addr, target string) (string, *bufio.Reader, net.Conn) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy %s: %v", addr, err)
	}
	proxyWritef(t, conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read CONNECT status: %v", err)
	}
	for {
		hdr, err := br.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read CONNECT headers: %v", err)
		}
		if hdr == "\r\n" || hdr == "\n" {
			break
		}
	}
	return status, br, conn
}

func proxyStatusCode(t *testing.T, status string) string {
	t.Helper()
	fields := strings.Fields(status)
	if len(fields) < 2 {
		t.Fatalf("malformed status line %q", status)
	}
	return fields[1]
}

func TestProxyAllowsListedHost(t *testing.T) {
	t.Parallel()

	const body = "tunnel bytes flow both ways"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	target := proxyHostPort(t, srv.URL)

	ctx := context.Background()
	addr, stop, err := ServeProxy(ctx, []string{target})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	status, br, conn := proxyConnect(t, addr, target)
	defer func() { _ = conn.Close() }()
	if code := proxyStatusCode(t, status); code != "200" {
		t.Fatalf("CONNECT to allowlisted %s: status %q, want 200", target, strings.TrimSpace(status))
	}
	// Tunnel a full request/response through: bytes flow both ways only if
	// the proxy forwards instead of answering itself.
	proxyWritef(t, conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", target)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read tunnelled response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read tunnelled body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(got) != body {
		t.Fatalf("tunnelled exchange = %d %q, want 200 %q", resp.StatusCode, got, body)
	}

	// Removing the entry makes the same CONNECT fail.
	deniedAddr, deniedStop, err := ServeProxy(ctx, nil)
	if err != nil {
		t.Fatalf("ServeProxy (empty allowlist): %v", err)
	}
	defer deniedStop()
	deniedStatus, _, deniedConn := proxyConnect(t, deniedAddr, target)
	defer func() { _ = deniedConn.Close() }()
	if code := proxyStatusCode(t, deniedStatus); code == "200" {
		t.Fatalf("CONNECT with the entry removed: status 200, want a denial")
	}
}

func TestProxyDeniesUnlistedHost(t *testing.T) {
	t.Parallel()

	// The "evil" target counts arrivals so the test can prove no bytes
	// leave the proxy on a denial.
	evil, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen evil target: %v", err)
	}
	defer func() { _ = evil.Close() }()
	arrived := make(chan net.Conn, 4)
	go func() {
		for {
			c, err := evil.Accept()
			if err != nil {
				return
			}
			arrived <- c
		}
	}()

	ctx := context.Background()
	addr, stop, err := ServeProxy(ctx, []string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	status, _, conn := proxyConnect(t, addr, evil.Addr().String())
	defer func() { _ = conn.Close() }()
	if code := proxyStatusCode(t, status); code != "403" {
		t.Fatalf("CONNECT to unlisted host: status %q, want 403", strings.TrimSpace(status))
	}
	select {
	case c := <-arrived:
		_ = c.Close()
		t.Fatal("denied CONNECT reached the target; no bytes may leave the proxy on a 403")
	case <-time.After(300 * time.Millisecond):
	}

	// Control: the target itself is reachable, so the denial came from
	// the proxy and not from a dead fixture.
	direct, err := net.DialTimeout("tcp", evil.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial target directly: %v", err)
	}
	defer func() { _ = direct.Close() }()
	var accepted net.Conn
	select {
	case accepted = <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("direct dial never arrived at the target")
	}
	defer func() { _ = accepted.Close() }()
	if _, err := direct.Write([]byte("ping")); err != nil {
		t.Fatalf("write direct: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(accepted, buf); err != nil {
		t.Fatalf("read direct: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("direct exchange got %q, want %q", buf, "ping")
	}
}

func TestProxyRejectsPlainHTTP(t *testing.T) {
	t.Parallel()

	var serverHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		serverHits.Add(1)
	}))
	defer srv.Close()
	target := proxyHostPort(t, srv.URL)

	ctx := context.Background()
	addr, stop, err := ServeProxy(ctx, []string{target})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()
	proxyWritef(t, conn, "GET http://%s/ HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", target, target)
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read GET status: %v", err)
	}
	if code := proxyStatusCode(t, status); code != "405" {
		t.Fatalf("plain GET: status %q, want 405", strings.TrimSpace(status))
	}
	// Drain the rejection so the handler goroutine can finish cleanly.
	_, _ = io.Copy(io.Discard, br)
	if hits := serverHits.Load(); hits != 0 {
		t.Fatalf("plain GET reached the target %d time(s); a 405 must never be forwarded", hits)
	}
}

func TestProxyEnvPinsAddress(t *testing.T) {
	t.Parallel()

	env := ProxyEnv("127.0.0.1:38231")
	wantKeys := []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"}
	if len(env) != len(wantKeys) {
		t.Fatalf("ProxyEnv returned %d entries %v, want exactly %v", len(env), env, wantKeys)
	}
	for _, k := range wantKeys {
		if env[k] != "http://127.0.0.1:38231" {
			t.Fatalf("ProxyEnv[%q] = %q, want %q", k, env[k], "http://127.0.0.1:38231")
		}
	}
}

func TestProxyRejectsOverlongRequestLine(t *testing.T) {
	t.Parallel()

	addr, stop, err := ServeProxy(context.Background(), []string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()
	// One line longer than the proxy buffers: it must be rejected with 431
	// without the proxy retaining an unbounded line in memory.
	proxyWritef(t, conn, "CONNECT %s HTTP/1.1\r\n\r\n", strings.Repeat("A", proxyLineLimit))
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if code := proxyStatusCode(t, status); code != "431" {
		t.Fatalf("overlong request line: status %q, want 431", strings.TrimSpace(status))
	}
}

func TestProxyRejectsOverlongHeaderLine(t *testing.T) {
	t.Parallel()

	addr, stop, err := ServeProxy(context.Background(), []string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()
	proxyWritef(t, conn, "CONNECT 127.0.0.1:1 HTTP/1.1\r\nX-Pad: %s\r\n\r\n", strings.Repeat("B", proxyLineLimit))
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if code := proxyStatusCode(t, status); code != "431" {
		t.Fatalf("overlong header line: status %q, want 431", strings.TrimSpace(status))
	}
}

func TestProxyDelivers431WithTrailingBody(t *testing.T) {
	t.Parallel()

	addr, stop, err := ServeProxy(context.Background(), []string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()
	// An oversized request line followed by a large still-arriving body:
	// closing with unread received data would reset the connection and
	// discard the 431, so the denial must drain before closing.
	proxyWritef(t, conn, "CONNECT %s HTTP/1.1\r\n%s", strings.Repeat("A", proxyLineLimit), strings.Repeat("C", 64<<10))
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read status after oversized request with trailing body: %v", err)
	}
	if code := proxyStatusCode(t, status); code != "431" {
		t.Fatalf("oversized request with trailing body: status %q, want 431", strings.TrimSpace(status))
	}
}

func TestProxyCancelClosesActiveTunnel(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	addr, stop, err := ServeProxy(ctx, []string{proxyHostPort(t, upstream.URL)})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()

	status, _, conn := proxyConnect(t, addr, proxyHostPort(t, upstream.URL))
	defer func() { _ = conn.Close() }()
	if code := proxyStatusCode(t, status); code != "200" {
		t.Fatalf("CONNECT to listed host: status %q, want 200", strings.TrimSpace(status))
	}
	cancel()

	// The active tunnel must not relay past cancellation: the proxy's
	// side closes, so a read observes EOF instead of hanging.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	one := make([]byte, 1)
	if _, err := io.ReadFull(conn, one); err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatalf("read after cancel: err %v, want EOF (tunnel still open)", err)
	}
}

func TestServeProxyStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	addr, stop, err := ServeProxy(ctx, []string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("ServeProxy: %v", err)
	}
	defer stop()
	cancel()

	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err != nil {
			return // Listener is closed: cancellation stopped the proxy.
		}
		_ = c.Close()
		if time.Now().After(deadline) {
			t.Fatal("proxy still accepts connections 5s after context cancellation")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
