package conn

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynfake "k8s.io/client-go/dynamic/fake"
)

// fakeCore serves typed core/v1 objects through a fake dynamic client.
func fakeCore(objs ...runtime.Object) coreClient {
	sch := runtime.NewScheme()
	corev1.AddToScheme(sch)
	return coreClient{dynfake.NewSimpleDynamicClient(sch, objs...)}
}

func readyPod(name string, ready bool, lbl map[string]string, ports ...corev1.ContainerPort) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "db", Labels: lbl},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "postgres", Ports: ports}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
}

func service(name string, sel map[string]string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "db"},
		Spec:       corev1.ServiceSpec{Selector: sel, Ports: ports},
	}
}

func TestK8sResolveServicePicksReadyPod(t *testing.T) {
	primary := map[string]string{"cnpg.io/cluster": "pg", "cnpg.io/instanceRole": "primary"}
	cs := fakeCore(
		service("pg-rw", primary, corev1.ServicePort{Port: 5432, TargetPort: intstr.FromString("postgresql")}),
		readyPod("pg-1", false, primary),
		readyPod("pg-2", true, primary, corev1.ContainerPort{Name: "postgresql", ContainerPort: 5432}),
		readyPod("pg-3", true, map[string]string{"cnpg.io/cluster": "pg", "cnpg.io/instanceRole": "replica"}),
	)
	f := &k8sForwarder{client: cs, target: K8sForward{Namespace: "db", CNPGCluster: "pg"}, port: 5432, service: "pg-rw"}

	pod, port, err := f.resolve(context.Background())
	if err != nil || pod != "pg-2" || port != 5432 {
		t.Fatalf("resolve() = %q, %d, %v; want the ready primary pg-2:5432", pod, port, err)
	}
	if got := f.remoteHost(); got != "pg-rw.db.svc" {
		t.Errorf("remoteHost() = %q, want the name in CNPG's certificate", got)
	}
}

func TestK8sResolveErrors(t *testing.T) {
	sel := map[string]string{"app": "db"}
	cs := fakeCore(
		service("only-unready", sel, corev1.ServicePort{Port: 3306, TargetPort: intstr.FromInt32(3306)}),
		readyPod("db-0", false, sel),
		service("two-ports", map[string]string{"x": "y"},
			corev1.ServicePort{Port: 80}, corev1.ServicePort{Port: 443}),
	)
	ctx := context.Background()

	f := &k8sForwarder{client: cs, target: K8sForward{Namespace: "db"}, port: 3306, service: "only-unready"}
	if _, _, err := f.resolve(ctx); !errors.Is(err, ErrK8sNoReadyPod) {
		t.Errorf("no ready pod: %v", err)
	}
	f.service = "two-ports"
	if _, _, err := f.resolve(ctx); !errors.Is(err, ErrK8sPortNotFound) || !strings.Contains(err.Error(), "80, 443") {
		t.Errorf("port not on service: %v", err)
	}
	f.service = "missing"
	if _, _, err := f.resolve(ctx); err == nil {
		t.Error("missing service should fail")
	}

	pod := &k8sForwarder{target: K8sForward{Namespace: "db", Pod: "db-0"}, port: 3307}
	if name, port, err := pod.resolve(ctx); name != "db-0" || port != 3307 || err != nil {
		t.Errorf("pod target = %q %d %v", name, port, err)
	}
}

func TestContainerPort(t *testing.T) {
	pod := readyPod("p", true, nil, corev1.ContainerPort{Name: "mysql", ContainerPort: 3306})
	cases := []struct {
		sp   corev1.ServicePort
		want int
	}{
		{corev1.ServicePort{Port: 3306, TargetPort: intstr.FromInt32(33060)}, 33060},
		{corev1.ServicePort{Port: 3306, TargetPort: intstr.FromString("mysql")}, 3306},
		{corev1.ServicePort{Port: 3306}, 3306},
	}
	for _, c := range cases {
		if got, err := containerPort(pod, c.sp); got != c.want || err != nil {
			t.Errorf("containerPort(%v) = %d, %v; want %d", c.sp.TargetPort, got, err, c.want)
		}
	}
	if _, err := containerPort(pod, corev1.ServicePort{Port: 1, TargetPort: intstr.FromString("nope")}); !errors.Is(err, ErrK8sPortNotFound) {
		t.Errorf("unknown named port: %v", err)
	}
}

func cnpgSecrets(superuser bool) []runtime.Object {
	objs := []runtime.Object{
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "pg-app", Namespace: "db"},
			Data: map[string][]byte{"username": []byte("app"), "password": []byte("s3cret"), "dbname": []byte("app")}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "pg-ca", Namespace: "db"},
			Data: map[string][]byte{"ca.crt": []byte("-----BEGIN CERTIFICATE-----\n")}},
	}
	if superuser {
		objs = append(objs, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "pg-superuser", Namespace: "db"},
			Data: map[string][]byte{"username": []byte("postgres"), "password": []byte("root"), "dbname": []byte("*")}})
	}
	return objs
}

func TestReadCNPGCredentials(t *testing.T) {
	ctx := context.Background()
	cs := fakeCore(cnpgSecrets(true)...)

	app, err := readCNPGCredentials(ctx, cs, "db", "pg", "")
	if err != nil || app.user != "app" || app.password != "s3cret" || app.database != "app" || len(app.caPEM) == 0 {
		t.Fatalf("app creds = %+v, %v", app, err)
	}
	su, err := readCNPGCredentials(ctx, cs, "db", "pg", CNPGUserSuperuser)
	if err != nil || su.user != "postgres" || su.database != "postgres" {
		t.Fatalf("superuser creds = %+v, %v (dbname * should become postgres)", su, err)
	}

	noSU := fakeCore(cnpgSecrets(false)...)
	_, err = readCNPGCredentials(ctx, noSU, "db", "pg", CNPGUserSuperuser)
	if !errors.Is(err, ErrCNPGSecret) || !strings.Contains(err.Error(), "enableSuperuserAccess") {
		t.Errorf("missing superuser secret: %v", err)
	}
	if _, err := readCNPGCredentials(ctx, fakeCore(cnpgSecrets(false)[0]), "db", "pg", ""); !errors.Is(err, ErrCNPGSecret) {
		t.Errorf("missing CA secret: %v", err)
	}
}

func TestPrepareK8sCNPG(t *testing.T) {
	fw := &k8sForwarder{client: fakeCore(cnpgSecrets(false)...), target: K8sForward{Namespace: "db", CNPGCluster: "pg"}, service: "pg-rw"}
	var opts openOptions
	c, err := prepareK8s(context.Background(), ConnectionConfig{Type: PostgreSQL, K8s: &fw.target}, fw, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "pg-rw.db.svc" || c.User != "app" || c.Password != "s3cret" || c.Database != "app" || c.SSLMode != "verify-full" || opts.caPEM == nil {
		t.Errorf("prepared = host %q user %q db %q ssl %q ca %v", c.Host, c.User, c.Database, c.SSLMode, opts.caPEM != nil)
	}

	c, _ = prepareK8s(context.Background(), ConnectionConfig{Type: PostgreSQL, Database: "other", K8s: &fw.target}, fw, &opts)
	if c.Database != "other" {
		t.Errorf("explicit database overridden: %q", c.Database)
	}
}

func TestListK8sTargets(t *testing.T) {
	sch := runtime.NewScheme()
	corev1.AddToScheme(sch)
	cluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1", "kind": "Cluster",
		"metadata": map[string]any{"name": "pg", "namespace": "db"},
		"spec":     map[string]any{"instances": int64(3)},
		"status":   map[string]any{"readyInstances": int64(2), "phase": "Cluster in healthy state"},
	}}
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(sch,
		map[schema.GroupVersionResource]string{cnpgClusterGVR: "ClusterList"},
		service("pg-rw", nil, corev1.ServicePort{Port: 5432}), cluster)

	got, err := listK8sTargets(context.Background(), dyn, "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "pg-rw" || got.Services[0].Ports[0] != 5432 {
		t.Errorf("services = %+v", got.Services)
	}
	want := CNPGCluster{Name: "pg", Instances: 3, ReadyInstances: 2, Phase: "Cluster in healthy state"}
	if len(got.CNPGClusters) != 1 || got.CNPGClusters[0] != want {
		t.Errorf("clusters = %+v", got.CNPGClusters)
	}
}

func TestValidateK8s(t *testing.T) {
	cnpg := &K8sForward{Namespace: "db", CNPGCluster: "pg"}
	ok := map[string]ConnectionConfig{
		"cnpg (no user/host needed)": {Type: PostgreSQL, K8s: cnpg},
		"service":                    {Type: MySQL, User: "root", K8s: &K8sForward{Namespace: "db", Service: "mysql"}},
		"pod":                        {Type: SQLServer, User: "sa", K8s: &K8sForward{Namespace: "db", Pod: "mssql-0", Port: 1433}},
	}
	for name, c := range ok {
		if err := c.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	bad := map[string]struct {
		cfg  ConnectionConfig
		want error
	}{
		"no namespace":        {ConnectionConfig{Type: PostgreSQL, K8s: &K8sForward{CNPGCluster: "pg"}}, ErrMissingField},
		"no target":           {ConnectionConfig{Type: PostgreSQL, User: "u", K8s: &K8sForward{Namespace: "db"}}, ErrInvalidK8sTarget},
		"two targets":         {ConnectionConfig{Type: PostgreSQL, User: "u", K8s: &K8sForward{Namespace: "db", Service: "a", Pod: "b"}}, ErrInvalidK8sTarget},
		"cnpg on mysql":       {ConnectionConfig{Type: MySQL, K8s: cnpg}, ErrUnsupportedOption},
		"with ssh":            {ConnectionConfig{Type: PostgreSQL, K8s: cnpg, SSH: &SSHTunnel{Host: "b", User: "u", Auth: SSHAuthAgent}}, ErrUnsupportedOption},
		"cnpg with tls":       {ConnectionConfig{Type: PostgreSQL, K8s: cnpg, TLS: &TLSFiles{CACertPath: "/ca.pem"}}, ErrUnsupportedOption},
		"service, no user":    {ConnectionConfig{Type: MySQL, K8s: &K8sForward{Namespace: "db", Service: "mysql"}}, ErrMissingField},
		"sqlite":              {ConnectionConfig{Type: SQLite, FilePath: "/x.db", K8s: cnpg}, ErrUnsupportedOption},
		"relative kubeconfig": {ConnectionConfig{Type: PostgreSQL, K8s: &K8sForward{Namespace: "db", CNPGCluster: "pg", KubeconfigPath: "kube/config"}}, ErrInvalidPath},
	}
	for name, tt := range bad {
		if err := tt.cfg.Validate(); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", name, err, tt.want)
		}
	}

	c := ConnectionConfig{Type: PostgreSQL, K8s: &K8sForward{Namespace: "db", CNPGCluster: "pg", KubeconfigPath: "/home/u/.kube/config"}}
	if paths := c.FilePaths(); len(paths) != 1 || paths[0] != "/home/u/.kube/config" {
		t.Errorf("kubeconfig must go through the file allowlist: %v", paths)
	}
}
