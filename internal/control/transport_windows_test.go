package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func ctx5(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testFrame(t *testing.T) []byte {
	t.Helper()
	f, err := Encode(Frame{OperationID: "op_SYNTHETIC1", Method: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func endpoint(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "control.sock")
}

// serveEcho accepts connections on l and answers each request with the same
// frame until l is closed. The returned function waits for the loop to end.
func serveEcho(t *testing.T, l Listener) (wait func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				f, err := c.Receive(context.Background())
				if err != nil {
					return
				}
				_ = c.Respond(f)
			}()
		}
	}()
	return func() { <-done }
}

// ownerOnlyDACL reports why the pipe's DACL is not exactly one allow entry
// for the current user on a protected DACL.
func ownerOnlyDACL(pipe string) error {
	sd, err := windows.GetNamedSecurityInfo(pipe, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("reading the pipe DACL: %w", err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("DACL is not protected: it inherits entries from the parent")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("reading the pipe DACL entries: %w", err)
	}
	if acl == nil {
		return errors.New("pipe has no DACL: everyone has access")
	}
	if acl.AceCount != 1 {
		return fmt.Errorf("DACL has %d entries, want 1", acl.AceCount)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		return err
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		return fmt.Errorf("DACL entry type %d is not an allow entry", ace.Header.AceType)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	got := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) //nolint:gosec // G103: the SID follows the fixed ACE header in memory
	if !got.Equals(user.User.Sid) {
		return fmt.Errorf("DACL grants %s, want the current user %s", got, user.User.Sid)
	}
	return nil
}

func TestPipeRoundTrip(t *testing.T) {
	tr, err := NewPipeTransport()
	if err != nil {
		t.Fatal(err)
	}
	path := endpoint(t)
	l, err := tr.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	wait := serveEcho(t, l)
	defer func() { _ = l.Close(); wait() }()

	if err := ownerOnlyDACL(l.Addr()); err != nil {
		t.Fatalf("pipe %s is not restricted to its user: %v", l.Addr(), err)
	}

	c, err := tr.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if !c.Peer().SameUser || c.Peer().PID <= 0 {
		t.Fatalf("server peer = %+v, want same user with a pid", c.Peer())
	}
	frame := testFrame(t)
	got, err := c.Request(ctx5(t), frame)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(frame) {
		t.Fatalf("reply = %q, want the echoed request %q", got, frame)
	}
}

// TestPipeDACLCheckRejectsWorldPipe keeps the broken input for the DACL
// assertion: a pipe open to Everyone must fail it.
func TestPipeDACLCheckRejectsWorldPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\mythhelm-test-world-%d`, time.Now().UnixNano())
	l, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;WD)"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if err := ownerOnlyDACL(name); err == nil {
		t.Fatal("ownerOnlyDACL accepted a pipe granting Everyone access")
	}
}

func TestPipeDialWithoutSupervisor(t *testing.T) {
	tr, err := NewPipeTransport()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Dial(endpoint(t)); !errors.Is(err, ErrNoSupervisor) {
		t.Fatalf("Dial with no listener = %v, want ErrNoSupervisor", err)
	}
}

func TestPipeListenRefusesSecondListener(t *testing.T) {
	tr, err := NewPipeTransport()
	if err != nil {
		t.Fatal(err)
	}
	path := endpoint(t)
	l, err := tr.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if second, err := tr.Listen(path); err == nil {
		_ = second.Close()
		t.Fatal("a second Listen on the same endpoint succeeded; the pipe name can be squatted")
	}
}

func TestPipePeerIsSameLogon(t *testing.T) {
	self, err := currentIdentity()
	if err != nil {
		t.Fatal(err)
	}
	otherLogon := self
	otherLogon.LogonID++
	otherUser := self
	otherUser.SID = "S-1-5-21-1-2-3-4"

	peerAs := func(id pipeIdentity) func(net.Conn, bool) (pipeIdentity, error) {
		return func(net.Conn, bool) (pipeIdentity, error) { return id, nil }
	}
	tests := []struct {
		name       string
		clientSees pipeIdentity // identity the client side reads for the server
		serverSees pipeIdentity // identity the listening side reads for the client
		refused    bool
	}{
		{"same logon session is served", self, self, false},
		{"client in another logon session is refused", self, otherLogon, true},
		{"client of another user is refused", self, otherUser, true},
		{"server in another logon session is refused by the client", otherLogon, self, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := endpoint(t)
			server := &pipeTransport{self: self, peerOf: peerAs(tt.serverSees)}
			client := &pipeTransport{self: self, peerOf: peerAs(tt.clientSees)}
			l, err := server.Listen(path)
			if err != nil {
				t.Fatal(err)
			}
			wait := serveEcho(t, l)
			defer func() { _ = l.Close(); wait() }()

			c, err := client.Dial(path)
			if tt.clientSees != self {
				if err == nil || !strings.Contains(err.Error(), codePermissionDenied) {
					t.Fatalf("Dial = %v, want %s", err, codePermissionDenied)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = c.Close() }()

			frame := testFrame(t)
			reply, err := c.Request(ctx5(t), frame)
			if err != nil {
				t.Fatal(err)
			}
			if !tt.refused {
				if string(reply) != string(frame) {
					t.Fatalf("reply = %q, want the echoed request", reply)
				}
				return
			}
			var denied permissionDenied
			if err := json.Unmarshal(reply[prefixLen:], &denied); err != nil {
				t.Fatalf("reply %q is not a refusal frame: %v", reply, err)
			}
			if denied.Code != codePermissionDenied {
				t.Fatalf("refusal code = %q, want %q", denied.Code, codePermissionDenied)
			}
		})
	}
}

func TestSpawnEnvCarriesWindowsLocators(t *testing.T) {
	t.Setenv("MYTHHELM_HOME", `C:\synthetic\home`)
	t.Setenv("TEMP", `C:\synthetic\temp`)
	t.Setenv("USERNAME", "synthetic")
	t.Setenv("LocalAppData", `C:\synthetic\local`)
	t.Setenv("MYTHHELM_SECRET_PROBE", "must-not-pass")

	env := strings.Join(spawnEnv(), "\n")
	for _, want := range []string{`MYTHHELM_HOME=C:\synthetic\home`, `TEMP=C:\synthetic\temp`, "USERNAME=synthetic", `LocalAppData=C:\synthetic\local`} {
		if !strings.Contains(env, want) {
			t.Errorf("spawn environment lacks %q:\n%s", want, env)
		}
	}
	if strings.Contains(env, "MYTHHELM_SECRET_PROBE") {
		t.Errorf("spawn environment passes an unlisted variable:\n%s", env)
	}
}
