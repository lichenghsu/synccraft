package conn

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	testSSHUser     = "ops"
	testSSHPassword = "s3cret pass"
)

// testSSHServer is an in-process SSH server that forwards direct-tcpip channels.
type testSSHServer struct {
	addr     string
	hostKeys []ssh.Signer

	mu         sync.Mutex
	authorized map[string]bool // marshalled public keys
	conns      []net.Conn
}

func startSSHServer(t *testing.T, hostKeys ...ssh.Signer) *testSSHServer {
	t.Helper()
	if len(hostKeys) == 0 {
		hostKeys = []ssh.Signer{newEd25519Signer(t)}
	}
	srv := &testSSHServer{hostKeys: hostKeys, authorized: map[string]bool{}}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == testSSHUser && string(pw) == testSSHPassword {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			srv.mu.Lock()
			defer srv.mu.Unlock()
			if c.User() == testSSHUser && srv.authorized[string(key.Marshal())] {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	for _, k := range hostKeys {
		cfg.AddHostKey(k)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.addr = ln.Addr().String()
	t.Cleanup(func() {
		ln.Close()
		srv.dropAll()
	})

	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			srv.mu.Lock()
			srv.conns = append(srv.conns, nc)
			srv.mu.Unlock()
			go srv.serve(nc, cfg)
		}
	}()
	return srv
}

func (s *testSSHServer) serve(nc net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		var network, addr string
		switch nch.ChannelType() {
		case "direct-tcpip":
			var req struct {
				Host       string
				Port       uint32
				OriginHost string
				OriginPort uint32
			}
			if err := ssh.Unmarshal(nch.ExtraData(), &req); err != nil {
				nch.Reject(ssh.Prohibited, "bad request")
				continue
			}
			network, addr = "tcp", net.JoinHostPort(req.Host, strconv.Itoa(int(req.Port)))
		case "direct-streamlocal@openssh.com":
			var req struct {
				SocketPath string
				Reserved0  string
				Reserved1  uint32
			}
			if err := ssh.Unmarshal(nch.ExtraData(), &req); err != nil {
				nch.Reject(ssh.Prohibited, "bad request")
				continue
			}
			network, addr = "unix", req.SocketPath
		default:
			nch.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		target, err := net.Dial(network, addr)
		if err != nil {
			nch.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		ch, creqs, err := nch.Accept()
		if err != nil {
			target.Close()
			continue
		}
		go ssh.DiscardRequests(creqs)
		go func() {
			defer ch.Close()
			defer target.Close()
			go io.Copy(target, ch)
			io.Copy(ch, target)
		}()
	}
}

func (s *testSSHServer) authorize(key ssh.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorized[string(key.Marshal())] = true
}

// dropAll simulates the network going away.
func (s *testSSHServer) dropAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.Close()
	}
	s.conns = nil
}

func (s *testSSHServer) hostPort(t *testing.T) (string, int) {
	host, port, _ := net.SplitHostPort(s.addr)
	p, _ := strconv.Atoi(port)
	return host, p
}

// knownHosts writes a known_hosts file trusting the given keys for the server.
func (s *testSSHServer) knownHosts(t *testing.T, keys ...ssh.PublicKey) string {
	t.Helper()
	var lines []string
	for _, k := range keys {
		lines = append(lines, knownhosts.Line([]string{knownhosts.Normalize(s.addr)}, k))
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (s *testSSHServer) tunnel(t *testing.T, knownHostsPath string, mod func(*SSHTunnel)) *SSHTunnel {
	host, port := s.hostPort(t)
	tun := &SSHTunnel{Host: host, Port: port, User: testSSHUser, Auth: SSHAuthPassword,
		Password: testSSHPassword, KnownHostsPath: knownHostsPath}
	if mod != nil {
		mod(tun)
	}
	return tun
}

func newEd25519Signer(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// startEcho returns the address of a TCP server that echoes what it reads.
func startEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return ln.Addr().String()
}

func assertEcho(t *testing.T, tun *sshTunnel, target string) {
	t.Helper()
	c, err := tun.DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("DialContext() = %v", err)
	}
	defer c.Close()
	msg := []byte("ping")
	if _, err := c.Write(msg); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(c, got); err != nil || string(got) != "ping" {
		t.Fatalf("echo = %q, %v", got, err)
	}
}

func openTunnel(t *testing.T, cfg *SSHTunnel) *sshTunnel {
	t.Helper()
	tun, err := newSSHTunnel(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newSSHTunnel() = %v", err)
	}
	t.Cleanup(func() { tun.Close() })
	return tun
}

func writeKey(t *testing.T, key ed25519.PrivateKey, passphrase string) string {
	t.Helper()
	var block *pem.Block
	var err error
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(key, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSSHPasswordAuth(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())
	assertEcho(t, openTunnel(t, srv.tunnel(t, kh, nil)), startEcho(t))
}

func TestSSHPrivateKeyAuth(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())
	target := startEcho(t)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	srv.authorize(signer.PublicKey())

	plain := writeKey(t, priv, "")
	assertEcho(t, openTunnel(t, srv.tunnel(t, kh, func(s *SSHTunnel) {
		s.Auth, s.Password, s.PrivateKeyPath = SSHAuthPrivateKey, "", plain
	})), target)

	encrypted := writeKey(t, priv, "key pass")
	assertEcho(t, openTunnel(t, srv.tunnel(t, kh, func(s *SSHTunnel) {
		s.Auth, s.Password, s.PrivateKeyPath, s.Passphrase = SSHAuthPrivateKey, "", encrypted, "key pass"
	})), target)

	for passphrase, want := range map[string]error{"": ErrSSHPassphraseRequired, "wrong": ErrSSHWrongPassphrase} {
		_, err := newSSHTunnel(context.Background(), srv.tunnel(t, kh, func(s *SSHTunnel) {
			s.Auth, s.PrivateKeyPath, s.Passphrase = SSHAuthPrivateKey, encrypted, passphrase
		}))
		if !errors.Is(err, want) {
			t.Errorf("passphrase %q: err = %v, want %v", passphrase, err, want)
		}
	}
}

func TestSSHAgentAuth(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	srv.authorize(signer.PublicKey())

	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, c)
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)

	assertEcho(t, openTunnel(t, srv.tunnel(t, kh, func(s *SSHTunnel) {
		s.Auth, s.Password = SSHAuthAgent, ""
	})), startEcho(t))

	t.Setenv("SSH_AUTH_SOCK", "")
	_, err = newSSHTunnel(context.Background(), srv.tunnel(t, kh, func(s *SSHTunnel) { s.Auth = SSHAuthAgent }))
	if !errors.Is(err, ErrSSHAgentUnavailable) {
		t.Errorf("no agent: err = %v, want ErrSSHAgentUnavailable", err)
	}
}

func TestSSHWrongPassword(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())
	_, err := newSSHTunnel(context.Background(), srv.tunnel(t, kh, func(s *SSHTunnel) { s.Password = "nope" }))
	if !errors.Is(err, ErrSSHAuthFailed) {
		t.Fatalf("err = %v, want ErrSSHAuthFailed", err)
	}
}

func TestSSHHostKeyVerification(t *testing.T) {
	srv := startSSHServer(t)
	other := newEd25519Signer(t)

	tests := map[string]struct {
		knownHosts string
		want       error
	}{
		"unknown host": {srv.knownHosts(t), ErrSSHHostUnknown},
		"changed key":  {srv.knownHosts(t, other.PublicKey()), ErrSSHHostKeyMismatch},
		"missing file": {filepath.Join(t.TempDir(), "nope"), ErrSSHKnownHostsMissing},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newSSHTunnel(context.Background(), srv.tunnel(t, tt.knownHosts, nil))
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// The server offers ed25519 first but known_hosts only has its ECDSA key;
// without restricting host key algorithms this would look like a mismatch.
func TestSSHUsesRecordedKeyType(t *testing.T) {
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecSigner, _ := ssh.NewSignerFromKey(ecKey)
	srv := startSSHServer(t, newEd25519Signer(t), ecSigner)

	kh := srv.knownHosts(t, ecSigner.PublicKey())
	assertEcho(t, openTunnel(t, srv.tunnel(t, kh, nil)), startEcho(t))
}

func TestSSHReconnects(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())
	target := startEcho(t)
	tun := openTunnel(t, srv.tunnel(t, kh, nil))

	assertEcho(t, tun, target)
	srv.dropAll()
	assertEcho(t, tun, target)
}

func TestSSHTunnelCloseNil(t *testing.T) {
	var tun *sshTunnel
	if err := tun.Close(); err != nil {
		t.Fatalf("nil Close() = %v", err)
	}
}

func TestAlgorithmsFor(t *testing.T) {
	if got := algorithmsFor(ssh.KeyAlgoRSA); len(got) != 3 || got[0] != ssh.KeyAlgoRSASHA512 {
		t.Errorf("rsa algorithms = %v", got)
	}
	if got := algorithmsFor(ssh.KeyAlgoED25519); len(got) != 1 {
		t.Errorf("ed25519 algorithms = %v", got)
	}
}

func TestSSHUnixSocket(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())

	sock := filepath.Join(t.TempDir(), "db.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()

	tun := openTunnel(t, srv.tunnel(t, kh, nil))
	c, err := tun.DialContext(context.Background(), "unix", sock)
	if err != nil {
		t.Fatalf("DialContext(unix) = %v", err)
	}
	defer c.Close()
	c.Write([]byte("ping"))
	got := make([]byte, 4)
	if _, err := io.ReadFull(c, got); err != nil || string(got) != "ping" {
		t.Fatalf("echo over unix socket = %q, %v", got, err)
	}
}

func TestSSHForwardErrors(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())
	tun := openTunnel(t, srv.tunnel(t, kh, nil))
	ctx := context.Background()

	_, err := tun.DialContext(ctx, "unix", filepath.Join(t.TempDir(), "missing.sock"))
	if !errors.Is(err, ErrSSHForwardRejected) || !strings.Contains(err.Error(), "SELECT @@socket") {
		t.Errorf("missing socket: %v", err)
	}

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().String()
	ln.Close()
	_, err = tun.DialContext(ctx, "tcp", closed)
	if !errors.Is(err, ErrSSHForwardRejected) || !strings.Contains(err.Error(), "as seen from the SSH server") {
		t.Errorf("closed port: %v", err)
	}

	// A refused forward must not tear down the working SSH connection.
	assertEcho(t, tun, startEcho(t))
}

func TestExplainForwardProhibited(t *testing.T) {
	err := explainForwardError("unix", "/run/mysqld/mysqld.sock", &ssh.OpenChannelError{Reason: ssh.Prohibited, Message: "administratively prohibited"})
	if !errors.Is(err, ErrSSHForwardRejected) || !strings.Contains(err.Error(), "AllowStreamLocalForwarding") {
		t.Errorf("prohibited unix: %v", err)
	}
	err = explainForwardError("tcp", "db:3306", &ssh.OpenChannelError{Reason: ssh.Prohibited})
	if !strings.Contains(err.Error(), "AllowTcpForwarding") {
		t.Errorf("prohibited tcp: %v", err)
	}
}

func listenUnixEcho(t *testing.T, path string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
}

func TestDetectSocket(t *testing.T) {
	srv := startSSHServer(t)
	kh := srv.knownHosts(t, srv.hostKeys[0].PublicKey())
	dir := t.TempDir()

	mysqlSock := filepath.Join(dir, "mysql.sock")
	listenUnixEcho(t, mysqlSock)
	pgDir := filepath.Join(dir, "pg")
	os.Mkdir(pgDir, 0o755)
	listenUnixEcho(t, filepath.Join(pgDir, ".s.PGSQL.5433"))

	saved := socketCandidates
	t.Cleanup(func() { socketCandidates = saved })
	socketCandidates = map[DBType][]string{
		MySQL:      {filepath.Join(dir, "missing.sock"), mysqlSock},
		PostgreSQL: {filepath.Join(dir, "nope"), pgDir},
	}

	tunnel := srv.tunnel(t, kh, nil)
	ctx := context.Background()

	got, err := DetectSocket(ctx, ConnectionConfig{Type: MySQL, SSH: tunnel})
	if err != nil || got != mysqlSock {
		t.Errorf("mysql: DetectSocket() = %q, %v; want %q", got, err, mysqlSock)
	}
	got, err = DetectSocket(ctx, ConnectionConfig{Type: PostgreSQL, Port: 5433, SSH: tunnel})
	if err != nil || got != pgDir {
		t.Errorf("postgres: DetectSocket() = %q, %v; want the directory %q", got, err, pgDir)
	}

	socketCandidates = map[DBType][]string{MySQL: {filepath.Join(dir, "missing.sock")}}
	if _, err := DetectSocket(ctx, ConnectionConfig{Type: MySQL, SSH: tunnel}); !errors.Is(err, ErrSocketNotFound) {
		t.Errorf("none found: %v, want ErrSocketNotFound", err)
	}
	if _, err := DetectSocket(ctx, ConnectionConfig{Type: MySQL}); !errors.Is(err, ErrUnsupportedOption) {
		t.Errorf("no tunnel: %v, want ErrUnsupportedOption", err)
	}
	if _, err := DetectSocket(ctx, ConnectionConfig{Type: SQLServer, SSH: tunnel}); !errors.Is(err, ErrUnsupportedOption) {
		t.Errorf("sqlserver: %v, want ErrUnsupportedOption", err)
	}
}
