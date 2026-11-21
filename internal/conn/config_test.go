package conn

import (
	"errors"
	"strings"
	"testing"
)

func validConfigs() map[string]ConnectionConfig {
	return map[string]ConnectionConfig{
		"mysql":     {Type: MySQL, Host: "127.0.0.1", Port: 3306, User: "root", Password: "root", Database: "shop"},
		"postgres":  {Type: PostgreSQL, Host: "127.0.0.1", User: "postgres", Password: "postgres", Database: "shop"},
		"sqlserver": {Type: SQLServer, Host: "127.0.0.1", User: "sa", Password: "Passw0rd!", TrustServerCertificate: true},
		"sqlite":    {Type: SQLite, FilePath: "/tmp/shop.db"},
	}
}

func TestValidate(t *testing.T) {
	for name, c := range validConfigs() {
		t.Run("ok/"+name, func(t *testing.T) {
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}

	base := validConfigs()
	tests := []struct {
		name string
		cfg  ConnectionConfig
		want error
	}{
		{"empty type", ConnectionConfig{Host: "h", User: "u"}, ErrUnsupportedType},
		{"unknown type", ConnectionConfig{Type: "oracle", Host: "h", User: "u"}, ErrUnsupportedType},
		{"mysql no host", with(base["mysql"], func(c *ConnectionConfig) { c.Host = "" }), ErrMissingField},
		{"mysql blank host", with(base["mysql"], func(c *ConnectionConfig) { c.Host = "  " }), ErrMissingField},
		{"postgres no user", with(base["postgres"], func(c *ConnectionConfig) { c.User = "" }), ErrMissingField},
		{"sqlite no path", ConnectionConfig{Type: SQLite}, ErrMissingField},
		{"negative port", with(base["mysql"], func(c *ConnectionConfig) { c.Port = -1 }), ErrInvalidPort},
		{"port too large", with(base["sqlserver"], func(c *ConnectionConfig) { c.Port = 70000 }), ErrInvalidPort},
		{"bad sslmode", with(base["postgres"], func(c *ConnectionConfig) { c.SSLMode = "on" }), ErrInvalidSSLMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateMissingFieldNamesField(t *testing.T) {
	c := validConfigs()["mysql"]
	c.Host = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("Validate() = %v, want error mentioning host", err)
	}
}

func TestKey(t *testing.T) {
	c := validConfigs()["mysql"]

	k := c.Key()
	if k == "" {
		t.Fatal("Key() is empty")
	}
	if k != c.Key() {
		t.Fatal("Key() is not deterministic")
	}
	if strings.Contains(k, c.Password) {
		t.Fatal("Key() leaks the password")
	}

	changes := map[string]func(*ConnectionConfig){
		"type":     func(c *ConnectionConfig) { c.Type = PostgreSQL },
		"host":     func(c *ConnectionConfig) { c.Host = "10.0.0.1" },
		"port":     func(c *ConnectionConfig) { c.Port = 3307 },
		"user":     func(c *ConnectionConfig) { c.User = "app" },
		"password": func(c *ConnectionConfig) { c.Password = "changed" },
		"database": func(c *ConnectionConfig) { c.Database = "other" },
		"filePath": func(c *ConnectionConfig) { c.FilePath = "/x.db" },
		"sslMode":  func(c *ConnectionConfig) { c.SSLMode = "require" },
		"trust":    func(c *ConnectionConfig) { c.TrustServerCertificate = true },
		"ssh":      func(c *ConnectionConfig) { c.SSH = &SSHTunnel{Host: "bastion", User: "u", Auth: SSHAuthAgent} },
		"tls":      func(c *ConnectionConfig) { c.TLS = &TLSFiles{CACertPath: "/ca.pem"} },
	}
	for field, change := range changes {
		changed := with(c, change)
		if changed.Key() == k {
			t.Errorf("Key() unchanged after changing %s", field)
		}
	}

	withSSH := with(c, func(c *ConnectionConfig) {
		c.SSH = &SSHTunnel{Host: "bastion", User: "u", Auth: SSHAuthPassword, Password: "ssh-secret"}
	})
	other := with(withSSH, func(c *ConnectionConfig) {
		c.SSH = &SSHTunnel{Host: "bastion", User: "u", Auth: SSHAuthPassword, Password: "ssh-changed"}
	})
	if withSSH.Key() == other.Key() {
		t.Error("Key() unchanged after changing ssh.password")
	}

	// Naive concatenation would make these collide: "ab"+"c" == "a"+"bc".
	a := ConnectionConfig{Type: MySQL, Host: "ab", User: "c"}
	b := ConnectionConfig{Type: MySQL, Host: "a", User: "bc"}
	if a.Key() == b.Key() {
		t.Error("Key() collides across field boundaries")
	}
}

func TestValidateSSH(t *testing.T) {
	base := validConfigs()["mysql"]
	tunnel := func(s SSHTunnel) ConnectionConfig {
		return with(base, func(c *ConnectionConfig) { c.SSH = &s })
	}

	ok := map[string]ConnectionConfig{
		"password":    tunnel(SSHTunnel{Host: "bastion", User: "ops", Auth: SSHAuthPassword, Password: "pw"}),
		"private key": tunnel(SSHTunnel{Host: "bastion", Port: 2222, User: "ops", Auth: SSHAuthPrivateKey, PrivateKeyPath: "/home/ops/.ssh/id_ed25519"}),
		"agent":       tunnel(SSHTunnel{Host: "bastion", User: "ops", Auth: SSHAuthAgent}),
	}
	for name, c := range ok {
		t.Run("ok/"+name, func(t *testing.T) {
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}

	tests := []struct {
		name string
		cfg  ConnectionConfig
		want error
	}{
		{"no host", tunnel(SSHTunnel{User: "ops", Auth: SSHAuthAgent}), ErrMissingField},
		{"no user", tunnel(SSHTunnel{Host: "bastion", Auth: SSHAuthAgent}), ErrMissingField},
		{"bad port", tunnel(SSHTunnel{Host: "bastion", Port: 70000, User: "ops", Auth: SSHAuthAgent}), ErrInvalidPort},
		{"no auth", tunnel(SSHTunnel{Host: "bastion", User: "ops"}), ErrInvalidSSHAuth},
		{"unknown auth", tunnel(SSHTunnel{Host: "bastion", User: "ops", Auth: "kerberos"}), ErrInvalidSSHAuth},
		{"key without path", tunnel(SSHTunnel{Host: "bastion", User: "ops", Auth: SSHAuthPrivateKey}), ErrMissingField},
		{"sqlite", ConnectionConfig{Type: SQLite, FilePath: "/x.db", SSH: &SSHTunnel{Host: "b", User: "u", Auth: SSHAuthAgent}}, ErrUnsupportedOption},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateTLS(t *testing.T) {
	cfgs := validConfigs()
	withTLS := func(name string, f TLSFiles, mod ...func(*ConnectionConfig)) ConnectionConfig {
		return with(cfgs[name], func(c *ConnectionConfig) {
			c.TLS = &f
			for _, m := range mod {
				m(c)
			}
		})
	}
	mutual := TLSFiles{CACertPath: "/ca.pem", ClientCertPath: "/client.pem", ClientKeyPath: "/client.key"}

	ok := map[string]ConnectionConfig{
		"mysql mutual":     withTLS("mysql", mutual),
		"postgres mutual":  withTLS("postgres", mutual, func(c *ConnectionConfig) { c.SSLMode = "verify-full" }),
		"sqlserver ca":     withTLS("sqlserver", TLSFiles{CACertPath: "/ca.pem"}),
		"postgres ca only": withTLS("postgres", TLSFiles{CACertPath: "/ca.pem"}),
	}
	for name, c := range ok {
		t.Run("ok/"+name, func(t *testing.T) {
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}

	tests := []struct {
		name string
		cfg  ConnectionConfig
		want error
	}{
		{"cert without key", withTLS("mysql", TLSFiles{ClientCertPath: "/client.pem"}), ErrMissingField},
		{"key without cert", withTLS("mysql", TLSFiles{ClientKeyPath: "/client.key"}), ErrMissingField},
		{"sqlserver client cert", withTLS("sqlserver", mutual), ErrUnsupportedOption},
		{"postgres sslmode disable", withTLS("postgres", mutual, func(c *ConnectionConfig) { c.SSLMode = "disable" }), ErrInvalidSSLMode},
		{"sqlite", with(cfgs["sqlite"], func(c *ConnectionConfig) { c.TLS = &TLSFiles{CACertPath: "/ca.pem"} }), ErrUnsupportedOption},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDefaultPort(t *testing.T) {
	want := map[DBType]int{MySQL: 3306, PostgreSQL: 5432, SQLServer: 1433, SQLite: 0, "oracle": 0}
	for typ, port := range want {
		if got := DefaultPort(typ); got != port {
			t.Errorf("DefaultPort(%q) = %d, want %d", typ, got, port)
		}
	}
}

func TestNilConfig(t *testing.T) {
	var c *ConnectionConfig
	if err := c.Validate(); !errors.Is(err, ErrMissingField) {
		t.Errorf("nil Validate() = %v, want ErrMissingField", err)
	}
	if k := c.Key(); k != "" {
		t.Errorf("nil Key() = %q, want empty", k)
	}
}

func TestValidatePortBounds(t *testing.T) {
	base := validConfigs()["mysql"]
	for _, port := range []int{0, 1, 65535} {
		c := with(base, func(c *ConnectionConfig) { c.Port = port })
		if err := c.Validate(); err != nil {
			t.Errorf("port %d: Validate() = %v, want nil", port, err)
		}
	}
	c := with(base, func(c *ConnectionConfig) {
		c.SSH = &SSHTunnel{Host: "bastion", Port: 0, User: "ops", Auth: SSHAuthAgent}
	})
	if err := c.Validate(); err != nil {
		t.Errorf("ssh port 0: Validate() = %v, want nil", err)
	}
}

func TestFilePaths(t *testing.T) {
	c := ConnectionConfig{
		Type: MySQL, Host: "h", User: "u",
		TLS: &TLSFiles{CACertPath: "/ca.pem", ClientCertPath: "/c.pem", ClientKeyPath: "/c.key"},
		SSH: &SSHTunnel{Host: "b", User: "u", Auth: SSHAuthPrivateKey, PrivateKeyPath: "/id", KnownHostsPath: "/kh"},
	}
	got := strings.Join(c.FilePaths(), ",")
	if want := "/ca.pem,/c.pem,/c.key,/id,/kh"; got != want {
		t.Errorf("FilePaths() = %q, want %q", got, want)
	}

	sqlite := ConnectionConfig{Type: SQLite, FilePath: "/shop.db"}
	if got := sqlite.FilePaths(); len(got) != 1 || got[0] != "/shop.db" {
		t.Errorf("sqlite FilePaths() = %v", got)
	}
	none := ConnectionConfig{Type: MySQL}
	if got := none.FilePaths(); got != nil {
		t.Errorf("no paths: FilePaths() = %v, want nil", got)
	}
}

func TestValidatePaths(t *testing.T) {
	cfgs := validConfigs()
	bad := map[string]ConnectionConfig{
		"relative sqlite":  {Type: SQLite, FilePath: "shop.db"},
		"traversal sqlite": {Type: SQLite, FilePath: "/data/../etc/shop.db"},
		"tilde key": with(cfgs["mysql"], func(c *ConnectionConfig) {
			c.SSH = &SSHTunnel{Host: "b", User: "u", Auth: SSHAuthPrivateKey, PrivateKeyPath: "~/.ssh/id_ed25519"}
		}),
		"relative ca": with(cfgs["postgres"], func(c *ConnectionConfig) { c.TLS = &TLSFiles{CACertPath: "certs/ca.pem"} }),
		"unclean known_hosts": with(cfgs["mysql"], func(c *ConnectionConfig) {
			c.SSH = &SSHTunnel{Host: "b", User: "u", Auth: SSHAuthAgent, KnownHostsPath: "/home/u//.ssh/known_hosts"}
		}),
	}
	for name, c := range bad {
		t.Run(name, func(t *testing.T) {
			if err := c.Validate(); !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Validate() = %v, want ErrInvalidPath", err)
			}
		})
	}
}

func with(c ConnectionConfig, f func(*ConnectionConfig)) ConnectionConfig {
	f(&c)
	return c
}
