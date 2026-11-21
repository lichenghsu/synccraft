package conn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

var (
	ErrK8sNoReadyPod   = errors.New("no ready pod behind the kubernetes service")
	ErrK8sForward      = errors.New("kubernetes port-forward failed")
	ErrCNPGSecret      = errors.New("cannot read CloudNativePG secret")
	ErrK8sPortNotFound = errors.New("port not found on the kubernetes service")
)

var cnpgClusterGVR = schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}

// k8sClients loads kubeconfig the way kubectl does.
func k8sClients(kubeconfigPath, kubeContext string) (*rest.Config, dynamic.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules,
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, dyn, nil
}

// k8sForwarder dials the database through the API server's pod port-forward.
// Each dial resolves the target pod again, so a CNPG failover is followed.
type k8sForwarder struct {
	config  *rest.Config
	client  coreClient
	target  K8sForward
	port    int // wanted remote port: target.Port, or the engine default
	service string

	newDialer func(u *url.URL) (httpstream.Dialer, error) // replaced in tests
}

func newK8sForwarder(c ConnectionConfig) (*k8sForwarder, error) {
	cfg, dyn, err := k8sClients(c.K8s.KubeconfigPath, c.K8s.Context)
	if err != nil {
		return nil, err
	}
	f := &k8sForwarder{config: cfg, client: coreClient{dyn}, target: *c.K8s, port: c.K8s.Port}
	if f.port == 0 {
		f.port = DefaultPort(c.Type)
	}
	f.service = c.K8s.Service
	if c.K8s.CNPGCluster != "" {
		f.service = c.K8s.CNPGCluster + "-rw"
	}
	f.newDialer = f.defaultDialer
	return f, nil
}

// remoteHost is the name the DSN uses: it matches the service's TLS
// certificate (CNPG issues <cluster>-rw.<namespace>.svc) and is never resolved
// locally, because every dial goes through the forwarder.
func (f *k8sForwarder) remoteHost() string {
	if f.service != "" {
		return f.service + "." + f.target.Namespace + ".svc"
	}
	return f.target.Pod
}

func (f *k8sForwarder) Close() error { return nil }

// DialContext ignores addr: the target is the configured pod or service.
func (f *k8sForwarder) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	pod, port, err := f.resolve(ctx)
	if err != nil {
		return nil, err
	}

	base, _, err := rest.DefaultServerUrlFor(f.config)
	if err != nil {
		return nil, err
	}
	u := base.JoinPath("api", "v1", "namespaces", f.target.Namespace, "pods", pod, "portforward")
	dialer, err := f.newDialer(u)
	if err != nil {
		return nil, err
	}

	type result struct {
		conn httpstream.Connection
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, proto, err := dialer.Dial(portforward.PortForwardProtocolV1Name)
		if err == nil && proto != portforward.PortForwardProtocolV1Name {
			conn.Close()
			err = fmt.Errorf("unexpected protocol %q", proto)
		}
		ch <- result{conn, err}
	}()
	var res result
	select {
	case res = <-ch:
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.conn != nil {
				r.conn.Close()
			}
		}()
		return nil, ctx.Err()
	}
	if res.err != nil {
		return nil, fmt.Errorf("%w: pod %s/%s: %w", ErrK8sForward, f.target.Namespace, pod, res.err)
	}
	return openForwardStreams(res.conn, port, pod)
}

func (f *k8sForwarder) defaultDialer(u *url.URL) (httpstream.Dialer, error) {
	transport, upgrader, err := spdy.RoundTripperFor(f.config)
	if err != nil {
		return nil, err
	}
	spdyDialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)
	// Newer API servers prefer WebSockets; fall back to SPDY like kubectl does.
	ws, err := portforward.NewSPDYOverWebsocketDialer(u, f.config)
	if err != nil {
		return spdyDialer, nil
	}
	return portforward.NewFallbackDialer(ws, spdyDialer, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	}), nil
}

// resolve returns the pod and container port to forward to.
func (f *k8sForwarder) resolve(ctx context.Context) (string, int, error) {
	ns := f.target.Namespace
	if f.service == "" {
		return f.target.Pod, f.port, nil
	}

	svc, err := f.client.service(ctx, ns, f.service)
	if err != nil {
		return "", 0, fmt.Errorf("service %s/%s: %w", ns, f.service, err)
	}
	sp, err := pickServicePort(svc, f.port)
	if err != nil {
		return "", 0, err
	}

	pods, err := f.client.pods(ctx, ns, labels.SelectorFromSet(svc.Spec.Selector).String())
	if err != nil {
		return "", 0, fmt.Errorf("pods for service %s/%s: %w", ns, f.service, err)
	}
	for i := range pods {
		pod := &pods[i]
		if !podReady(pod) {
			continue
		}
		port, err := containerPort(pod, sp)
		if err != nil {
			return "", 0, err
		}
		return pod.Name, port, nil
	}
	return "", 0, fmt.Errorf("%w: %s/%s", ErrK8sNoReadyPod, ns, f.service)
}

func pickServicePort(svc *corev1.Service, want int) (corev1.ServicePort, error) {
	for _, p := range svc.Spec.Ports {
		if int(p.Port) == want {
			return p, nil
		}
	}
	if len(svc.Spec.Ports) == 1 {
		return svc.Spec.Ports[0], nil
	}
	var have []string
	for _, p := range svc.Spec.Ports {
		have = append(have, strconv.Itoa(int(p.Port)))
	}
	return corev1.ServicePort{}, fmt.Errorf("%w: %d on %s (has %s)", ErrK8sPortNotFound, want, svc.Name, strings.Join(have, ", "))
}

// containerPort maps a service port to the pod's port, resolving named targetPorts.
func containerPort(pod *corev1.Pod, sp corev1.ServicePort) (int, error) {
	tp := sp.TargetPort
	if tp.IntValue() != 0 {
		return tp.IntValue(), nil
	}
	if tp.String() == "" || tp.String() == "0" {
		return int(sp.Port), nil
	}
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.Name == tp.String() {
				return int(p.ContainerPort), nil
			}
		}
	}
	return 0, fmt.Errorf("%w: named port %q on pod %s", ErrK8sPortNotFound, tp.String(), pod.Name)
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// openForwardStreams opens the error and data streams for one forwarded
// connection, as kubectl does per local connection.
func openForwardStreams(conn httpstream.Connection, port int, pod string) (net.Conn, error) {
	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(port))
	headers.Set(corev1.PortForwardRequestIDHeader, "0")
	errStream, err := conn.CreateStream(headers)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: error stream: %w", ErrK8sForward, err)
	}
	errStream.Close() // we never write to it

	errCh := make(chan error, 1)
	go func() {
		msg, err := io.ReadAll(errStream)
		switch {
		case err != nil:
			errCh <- err
		case len(msg) > 0:
			errCh <- fmt.Errorf("%w: pod %s port %d: %s", ErrK8sForward, pod, port, msg)
		default:
			errCh <- nil
		}
	}()

	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	data, err := conn.CreateStream(headers)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: data stream: %w", ErrK8sForward, err)
	}
	return &forwardConn{data: data, conn: conn, errCh: errCh, pod: pod}, nil
}

// forwardConn adapts a port-forward data stream to net.Conn.
type forwardConn struct {
	data  httpstream.Stream
	conn  httpstream.Connection
	errCh chan error
	pod   string
	once  sync.Once
}

func (c *forwardConn) Read(b []byte) (int, error) {
	n, err := c.data.Read(b)
	if err == io.EOF {
		// The kubelet reports why the stream ended (e.g. nothing listening) on the error stream.
		select {
		case ferr := <-c.errCh:
			if ferr != nil {
				return n, ferr
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return n, err
}

func (c *forwardConn) Write(b []byte) (int, error) { return c.data.Write(b) }

func (c *forwardConn) Close() error {
	var err error
	c.once.Do(func() {
		c.data.Reset()
		err = c.conn.Close()
	})
	return err
}

func (c *forwardConn) LocalAddr() net.Addr  { return forwardAddr("synccraft") }
func (c *forwardConn) RemoteAddr() net.Addr { return forwardAddr(c.pod) }

// Deadlines are not supported by the stream; drivers fall back to closing the conn.
func (c *forwardConn) SetDeadline(time.Time) error      { return nil }
func (c *forwardConn) SetReadDeadline(time.Time) error  { return nil }
func (c *forwardConn) SetWriteDeadline(time.Time) error { return nil }

type forwardAddr string

func (a forwardAddr) Network() string { return "k8s-portforward" }
func (a forwardAddr) String() string  { return string(a) }

// prepareK8s points c at the forwarder's target and, for CNPG, fills in the
// credentials and CA from the cluster's secrets. They live only in memory.
func prepareK8s(ctx context.Context, c ConnectionConfig, fw *k8sForwarder, opts *openOptions) (ConnectionConfig, error) {
	c.Host = fw.remoteHost()
	if fw.target.CNPGCluster == "" {
		return c, nil
	}
	creds, err := readCNPGCredentials(ctx, fw.client, fw.target.Namespace, fw.target.CNPGCluster, fw.target.CNPGUser)
	if err != nil {
		return c, err
	}
	c.User, c.Password = creds.user, creds.password
	if c.Database == "" {
		c.Database = creds.database
	}
	c.SSLMode = "verify-full"
	opts.caPEM = creds.caPEM
	return c, nil
}

// cnpgCredentials reads what a CNPG cluster publishes for clients.
type cnpgCredentials struct {
	user, password, database string
	caPEM                    []byte
}

func readCNPGCredentials(ctx context.Context, cs coreClient, ns, cluster string, who CNPGUser) (cnpgCredentials, error) {
	if who == "" {
		who = CNPGUserApp
	}
	name := cluster + "-" + string(who)
	sec, err := cs.secret(ctx, ns, name)
	if err != nil {
		hint := ""
		if apierrors.IsNotFound(err) && who == CNPGUserSuperuser {
			hint = " (set enableSuperuserAccess: true on the Cluster)"
		}
		return cnpgCredentials{}, fmt.Errorf("%w %s/%s%s: %w", ErrCNPGSecret, ns, name, hint, err)
	}
	creds := cnpgCredentials{
		user:     string(sec.Data["username"]),
		password: string(sec.Data["password"]),
		database: string(sec.Data["dbname"]),
	}
	if creds.database == "" || creds.database == "*" {
		creds.database = "postgres"
	}

	caName := cluster + "-ca"
	ca, err := cs.secret(ctx, ns, caName)
	if err != nil {
		return cnpgCredentials{}, fmt.Errorf("%w %s/%s: %w", ErrCNPGSecret, ns, caName, err)
	}
	creds.caPEM = ca.Data["ca.crt"]
	if len(creds.caPEM) == 0 {
		return cnpgCredentials{}, fmt.Errorf("%w %s/%s: no ca.crt", ErrCNPGSecret, ns, caName)
	}
	return creds, nil
}

// K8sContexts lists the contexts in a kubeconfig.
type K8sContexts struct {
	Contexts []string `json:"contexts"`
	Current  string   `json:"current"`
}

func ListK8sContexts(kubeconfigPath string) (K8sContexts, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	raw, err := rules.Load()
	if err != nil {
		return K8sContexts{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	out := K8sContexts{Contexts: []string{}, Current: raw.CurrentContext}
	for name := range raw.Contexts {
		out.Contexts = append(out.Contexts, name)
	}
	slices.Sort(out.Contexts)
	return out, nil
}

func ListK8sNamespaces(ctx context.Context, kubeconfigPath, kubeContext string) ([]string, error) {
	_, dyn, err := k8sClients(kubeconfigPath, kubeContext)
	if err != nil {
		return nil, err
	}
	list, err := coreClient{dyn}.namespaces(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, n := range list {
		names = append(names, n.Name)
	}
	slices.Sort(names)
	return names, nil
}

// K8sTargets are the things in a namespace a connection can point at.
type K8sTargets struct {
	CNPGClusters []CNPGCluster `json:"cnpgClusters"`
	Services     []K8sService  `json:"services"`
}

type CNPGCluster struct {
	Name           string `json:"name"`
	Instances      int    `json:"instances"`
	ReadyInstances int    `json:"readyInstances"`
	Phase          string `json:"phase"`
}

type K8sService struct {
	Name  string `json:"name"`
	Ports []int  `json:"ports"`
}

func ListK8sTargets(ctx context.Context, kubeconfigPath, kubeContext, namespace string) (K8sTargets, error) {
	_, dyn, err := k8sClients(kubeconfigPath, kubeContext)
	if err != nil {
		return K8sTargets{}, err
	}
	return listK8sTargets(ctx, dyn, namespace)
}

func listK8sTargets(ctx context.Context, dyn dynamic.Interface, ns string) (K8sTargets, error) {
	out := K8sTargets{CNPGClusters: []CNPGCluster{}, Services: []K8sService{}}

	svcs, err := coreClient{dyn}.services(ctx, ns)
	if err != nil {
		return out, err
	}
	for _, s := range svcs {
		ks := K8sService{Name: s.Name, Ports: []int{}}
		for _, p := range s.Spec.Ports {
			ks.Ports = append(ks.Ports, int(p.Port))
		}
		out.Services = append(out.Services, ks)
	}

	clusters, err := dyn.Resource(cnpgClusterGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return out, fmt.Errorf("list CloudNativePG clusters: %w", err)
	}
	if clusters != nil {
		for _, u := range clusters.Items {
			c := CNPGCluster{Name: u.GetName()}
			if spec, ok := u.Object["spec"].(map[string]any); ok {
				if v, ok := spec["instances"].(int64); ok {
					c.Instances = int(v)
				}
			}
			if st, ok := u.Object["status"].(map[string]any); ok {
				if v, ok := st["readyInstances"].(int64); ok {
					c.ReadyInstances = int(v)
				}
				c.Phase, _ = st["phase"].(string)
			}
			out.CNPGClusters = append(out.CNPGClusters, c)
		}
	}
	return out, nil
}
