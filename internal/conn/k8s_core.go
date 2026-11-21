package conn

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// coreClient reads the few core/v1 objects we need through the dynamic client.
// The typed client would pull in client-go's global scheme, which registers
// every API group and adds ~14 MB to the binary.
type coreClient struct {
	dyn dynamic.Interface
}

var (
	podsGVR       = corev1.SchemeGroupVersion.WithResource("pods")
	servicesGVR   = corev1.SchemeGroupVersion.WithResource("services")
	secretsGVR    = corev1.SchemeGroupVersion.WithResource("secrets")
	namespacesGVR = corev1.SchemeGroupVersion.WithResource("namespaces")
)

func getAs[T any](ctx context.Context, dyn dynamic.Interface, gvr schema.GroupVersionResource, ns, name string) (*T, error) {
	u, err := dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	out := new(T)
	return out, runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, out)
}

func listAs[T any](ctx context.Context, dyn dynamic.Interface, gvr schema.GroupVersionResource, ns string, opts metav1.ListOptions) ([]T, error) {
	list, err := dyn.Resource(gvr).Namespace(ns).List(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(list.Items))
	for i := range list.Items {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c coreClient) service(ctx context.Context, ns, name string) (*corev1.Service, error) {
	return getAs[corev1.Service](ctx, c.dyn, servicesGVR, ns, name)
}

func (c coreClient) secret(ctx context.Context, ns, name string) (*corev1.Secret, error) {
	return getAs[corev1.Secret](ctx, c.dyn, secretsGVR, ns, name)
}

func (c coreClient) pods(ctx context.Context, ns, selector string) ([]corev1.Pod, error) {
	return listAs[corev1.Pod](ctx, c.dyn, podsGVR, ns, metav1.ListOptions{LabelSelector: selector})
}

func (c coreClient) services(ctx context.Context, ns string) ([]corev1.Service, error) {
	return listAs[corev1.Service](ctx, c.dyn, servicesGVR, ns, metav1.ListOptions{})
}

func (c coreClient) namespaces(ctx context.Context) ([]corev1.Namespace, error) {
	return listAs[corev1.Namespace](ctx, c.dyn, namespacesGVR, "", metav1.ListOptions{})
}
