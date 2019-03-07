package handlers

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations/class"
)

var _ handler.EventHandler = (*EnqueueRequestsForServiceEvent)(nil)

type EnqueueRequestsForServiceEvent struct {
	ServiceClass string

	Cache cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForServiceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfServiceClassMatched(e.Object.(*corev1.Service), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForServiceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfServiceClassMatched(e.ObjectOld.(*corev1.Service), queue)
	h.enqueueIfServiceClassMatched(e.ObjectNew.(*corev1.Service), queue)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForServiceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfServiceClassMatched(e.Object.(*corev1.Service), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForServiceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueIfServiceClassMatched(e.Object.(*corev1.Service), queue)
}

//TODO: this can be further optimized to only included serviceses referenced this service :D
func (h *EnqueueRequestsForServiceEvent) enqueueIfServiceClassMatched(service *corev1.Service, queue workqueue.RateLimitingInterface) {
	if !class.IsValidService(h.ServiceClass, service) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: service.Namespace,
			Name:      service.Name,
		},
	})
}
