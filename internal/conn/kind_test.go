//go:build kind

package conn

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// Needs dev/kind/setup.sh. Run: go test -tags kind ./internal/conn/...
const (
	kindContext   = "kind-synccraft"
	kindNamespace = "synccraft"
)

func kindCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestKindDiscovery(t *testing.T) {
	ctx := kindCtx(t)
	contexts, err := ListK8sContexts("")
	if err != nil || !slices.Contains(contexts.Contexts, kindContext) {
		t.Fatalf("contexts = %+v, %v (is dev/kind/setup.sh done?)", contexts, err)
	}
	ns, err := ListK8sNamespaces(ctx, "", kindContext)
	if err != nil || !slices.Contains(ns, kindNamespace) {
		t.Fatalf("namespaces = %v, %v", ns, err)
	}
	targets, err := ListK8sTargets(ctx, "", kindContext, kindNamespace)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets.CNPGClusters) != 1 || targets.CNPGClusters[0].Name != "pg" || targets.CNPGClusters[0].ReadyInstances != 1 {
		t.Errorf("clusters = %+v", targets.CNPGClusters)
	}
	if !slices.ContainsFunc(targets.Services, func(s K8sService) bool { return s.Name == "pg-rw" }) {
		t.Errorf("services = %+v", targets.Services)
	}
}

func TestKindCNPG(t *testing.T) {
	for _, who := range []CNPGUser{CNPGUserApp, CNPGUserSuperuser} {
		t.Run(string(who), func(t *testing.T) {
			ctx := kindCtx(t)
			c := ConnectionConfig{Type: PostgreSQL, K8s: &K8sForward{Context: kindContext, Namespace: kindNamespace, CNPGCluster: "pg", CNPGUser: who}}
			info, err := Probe(ctx, c)
			if err != nil || info.Product != ProductPostgreSQL {
				t.Fatalf("Probe() = %+v, %v", info, err)
			}
			o, err := open(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			defer o.Close()
			dbs, err := ListDatabases(ctx, o.db, PostgreSQL)
			if err != nil || !slices.Contains(dbs, "app") {
				t.Fatalf("ListDatabases() = %v, %v", dbs, err)
			}
		})
	}
}

// Generic targets with credentials typed in (read from the CNPG secret here).
func TestKindServiceAndPod(t *testing.T) {
	ctx := kindCtx(t)
	_, dyn, err := k8sClients("", kindContext)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := readCNPGCredentials(ctx, coreClient{dyn}, kindNamespace, "pg", CNPGUserApp)
	if err != nil {
		t.Fatal(err)
	}

	targets := map[string]K8sForward{
		"service": {Context: kindContext, Namespace: kindNamespace, Service: "pg-plain"},
		"pod":     {Context: kindContext, Namespace: kindNamespace, Pod: "pg-1"},
	}
	for name, k := range targets {
		t.Run(name, func(t *testing.T) {
			k := k
			c := ConnectionConfig{Type: PostgreSQL, User: creds.user, Password: creds.password, Database: "app", SSLMode: "require", K8s: &k}
			if err := Ping(kindCtx(t), c); err != nil {
				t.Fatalf("Ping() = %v", err)
			}
		})
	}

	t.Run("nothing listening", func(t *testing.T) {
		c := ConnectionConfig{Type: PostgreSQL, User: "app", K8s: &K8sForward{Context: kindContext, Namespace: kindNamespace, Pod: "pg-1", Port: 9999}}
		if err := Ping(kindCtx(t), c); !errors.Is(err, ErrK8sForward) {
			t.Fatalf("err = %v, want ErrK8sForward", err)
		}
	})
}

// Each dial resolves the primary again, so pooled connections keep working
// while connections are opened and closed repeatedly.
func TestKindPoolReuse(t *testing.T) {
	ctx := kindCtx(t)
	p := NewPool()
	defer p.CloseAll()
	c := ConnectionConfig{Type: PostgreSQL, K8s: &K8sForward{Context: kindContext, Namespace: kindNamespace, CNPGCluster: "pg"}}
	db, err := p.Get(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		var one int
		if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
			t.Fatalf("query %d: %v", i, err)
		}
	}
}
