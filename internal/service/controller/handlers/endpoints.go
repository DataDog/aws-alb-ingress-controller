package handlers

import (
	"context"
	"reflect"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations/class"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = (*EnqueueRequestsForEndpointsEvent)(nil)

type EnqueueRequestsForEndpointsEvent struct {
	ServiceClass string
	Cache        cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedServices(e.Object.(*corev1.Endpoints), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !reflect.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedServices(epNew, queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedServices(e.Object.(*corev1.Endpoints), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForEndpointsEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

//TODO: this can be further optimized to only included ingresses referenced this endpoints(service) :D
func (h *EnqueueRequestsForEndpointsEvent) enqueueImpactedServices(endpoints *corev1.Endpoints, queue workqueue.RateLimitingInterface) {
	serviceList := &corev1.ServiceList{}
	if err := h.Cache.List(context.Background(), client.InNamespace(endpoints.Namespace), serviceList); err != nil {
		glog.Errorf("failed to fetch impacted services by endpoints due to %v", err)
		return
	}

	for _, ingress := range serviceList.Items {
		if !class.IsValidService(h.ServiceClass, &ingress) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      ingress.Name,
			},
		})
	}
}
