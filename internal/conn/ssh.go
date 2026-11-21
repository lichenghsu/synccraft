package conn

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	ErrSSHHostUnknown        = errors.New("ssh host key is not in known_hosts")
	ErrSSHHostKeyMismatch    = errors.New("ssh host key does not match known_hosts")
	ErrSSHKnownHostsMissing  = errors.New("known_hosts file not found")
	ErrSSHAuthFailed         = errors.New("ssh authentication failed")
	ErrSSHPassphraseRequired = errors.New("ssh private key is encrypted; a passphrase is required")
	ErrSSHWrongPassphrase    = errors.New("wrong passphrase for ssh private key")
	ErrSSHAgentUnavailable   = errors.New("ssh agent not available (SSH_AUTH_SOCK is not set)")
	ErrSSHForwardRejected    = errors.New("ssh server refused the forward")
)

// sshTunnel dials database connections through one SSH client, reconnecting
// once if the client has died.
type sshTunnel struct {
	addr   string
	config *ssh.ClientConfig
	closer io.Closer // ssh-agent connection, if any

	mu     sync.Mutex
	client *ssh.Client
}

func newSSHTunnel(ctx context.Context, t *SSHTunnel) (*sshTunnel, error) {
	port := t.Port
	if port == 0 {
		port = DefaultSSHPort
	}
	addr := net.JoinHostPort(t.Host, strconv.Itoa(port))

	auth, closer, err := sshAuth(t)
	if err != nil {
		return nil, err
	}
	hostKeyCallback, algos, err := hostKeyCheck(t.KnownHostsPath, addr)
	if err != nil {
		closeQuietly(closer)
		return nil, err
	}

	tun := &sshTunnel{
		addr:   addr,
		closer: closer,
		config: &ssh.ClientConfig{
			User:              t.User,
			Auth:              auth,
			HostKeyCallback:   hostKeyCallback,
			HostKeyAlgorithms: algos,
			Timeout:           PingTimeout,
		},
	}
	// Connect now so auth and host key problems surface on "Test connection".
	if _, err := tun.get(ctx); err != nil {
		tun.Close()
		return nil, err
	}
	return tun, nil
}

// DialContext opens a connection to addr as seen from the SSH server.
func (t *sshTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := t.dial(ctx, network, addr)
	if err != nil {
		return nil, explainForwardError(network, addr, err)
	}
	return conn, nil
}

func (t *sshTunnel) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	client, err := t.get(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := client.DialContext(ctx, network, addr)
	if err == nil {
		return conn, nil
	}
	var rejected *ssh.OpenChannelError
	if errors.As(err, &rejected) {
		return nil, err // the server answered, so the SSH connection is fine
	}

	// The SSH connection may have dropped (sleep, network change): retry once.
	t.drop(client)
	client, rerr := t.get(ctx)
	if rerr != nil {
		return nil, rerr
	}
	return client.DialContext(ctx, network, addr)
}

// explainForwardError turns the SSH server's refusal to open a forward into
// something actionable.
func explainForwardError(network, addr string, err error) error {
	var rejected *ssh.OpenChannelError
	if !errors.As(err, &rejected) {
		return err
	}
	if rejected.Reason == ssh.Prohibited {
		option := "AllowTcpForwarding"
		if network == "unix" {
			option = "AllowStreamLocalForwarding"
		}
		return fmt.Errorf("%w: the SSH server does not allow forwarding to %s; enable %s in its sshd_config (%w)", ErrSSHForwardRejected, addr, option, err)
	}
	if network == "unix" {
		return fmt.Errorf("%w: the SSH server could not open the socket %s; check the path (e.g. run SELECT @@socket on the server) and that sshd allows AllowStreamLocalForwarding (%w)", ErrSSHForwardRejected, addr, err)
	}
	return fmt.Errorf("%w: the SSH server could not connect to %s; check the host and port as seen from the SSH server (%w)", ErrSSHForwardRejected, addr, err)
}

func (t *sshTunnel) get(ctx context.Context) (*ssh.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client != nil {
		return t.client, nil
	}

	d := net.Dialer{Timeout: PingTimeout}
	nc, err := d.DialContext(ctx, "tcp", t.addr)
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", t.addr, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		nc.SetDeadline(dl)
	}
	c, chans, reqs, err := ssh.NewClientConn(nc, t.addr, t.config)
	if err != nil {
		nc.Close()
		return nil, explainSSHError(t.addr, err)
	}
	nc.SetDeadline(time.Time{})

	t.client = ssh.NewClient(c, chans, reqs)
	return t.client, nil
}

func (t *sshTunnel) drop(c *ssh.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client == c {
		t.client.Close()
		t.client = nil
	}
}

func (t *sshTunnel) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var err error
	if t.client != nil {
		err = t.client.Close()
		t.client = nil
	}
	closeQuietly(t.closer)
	t.closer = nil
	return err
}

// hostDialer lets go-mssqldb resolve the database host at the tunnel's far end.
type hostDialer struct {
	dialer
	host string
}

func (h hostDialer) HostName() string { return h.host }

func sshAuth(t *SSHTunnel) ([]ssh.AuthMethod, io.Closer, error) {
	switch t.Auth {
	case SSHAuthPassword:
		answer := func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = t.Password
			}
			return answers, nil
		}
		return []ssh.AuthMethod{ssh.Password(t.Password), ssh.KeyboardInteractive(answer)}, nil, nil

	case SSHAuthPrivateKey:
		signer, err := loadPrivateKey(t.PrivateKeyPath, t.Passphrase)
		if err != nil {
			return nil, nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil, nil

	case SSHAuthAgent:
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, nil, ErrSSHAgentUnavailable
		}
		c, err := net.Dial("unix", sock)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrSSHAgentUnavailable, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(c).Signers)}, c, nil
	}
	return nil, nil, fmt.Errorf("%w: %q", ErrInvalidSSHAuth, t.Auth)
}

func loadPrivateKey(path, passphrase string) (ssh.Signer, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ssh private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(pem)
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		return signer, nil
	}

	if passphrase == "" {
		return nil, ErrSSHPassphraseRequired
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(passphrase))
	if errors.Is(err, x509.IncorrectPasswordError) {
		return nil, ErrSSHWrongPassphrase
	}
	if err != nil {
		return nil, fmt.Errorf("parse ssh private key: %w", err)
	}
	return signer, nil
}

// hostKeyCheck builds a strict known_hosts callback, plus the host key
// algorithms already recorded for addr. Without that list the server may
// present a key type we have no entry for, which looks like a mismatch.
func hostKeyCheck(path, addr string) (ssh.HostKeyCallback, []string, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: %s", ErrSSHKnownHostsMissing, path)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read known_hosts: %w", err)
	}

	strict := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			fp := key.Type() + " " + ssh.FingerprintSHA256(key)
			if len(keyErr.Want) == 0 {
				return fmt.Errorf("%w: %s presented %s; connect once with ssh to verify and add it", ErrSSHHostUnknown, hostname, fp)
			}
			return fmt.Errorf("%w: %s presented %s; the server was reinstalled or someone is intercepting the connection", ErrSSHHostKeyMismatch, hostname, fp)
		}
		var revoked *knownhosts.RevokedError
		if errors.As(err, &revoked) {
			return fmt.Errorf("%w: %s uses a revoked key", ErrSSHHostKeyMismatch, hostname)
		}
		return err
	}
	return strict, knownAlgorithms(cb, addr), nil
}

func knownAlgorithms(cb ssh.HostKeyCallback, addr string) []string {
	probe, _ := ssh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
	var keyErr *knownhosts.KeyError
	if err := cb(addr, &net.TCPAddr{}, probe); !errors.As(err, &keyErr) {
		return nil
	}

	var algos []string
	seen := map[string]bool{}
	for _, k := range keyErr.Want {
		for _, a := range algorithmsFor(k.Key.Type()) {
			if !seen[a] {
				seen[a] = true
				algos = append(algos, a)
			}
		}
	}
	return algos
}

func algorithmsFor(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{keyType}
}

func explainSSHError(addr string, err error) error {
	msg := err.Error()
	// "message type 51" is a USERAUTH_FAILURE where x/crypto expected a
	// keyboard-interactive prompt, e.g. OpenSSH without PAM rejecting a password.
	if strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "unexpected message type 51") {
		return fmt.Errorf("%w: %s: %w", ErrSSHAuthFailed, addr, err)
	}
	return fmt.Errorf("ssh %s: %w", addr, err)
}

func closeQuietly(c io.Closer) {
	if c != nil {
		c.Close()
	}
}
