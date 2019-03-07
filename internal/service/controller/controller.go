package controller

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/controller/handlers"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/generator"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Initialize(config *config.Configuration, mgr manager.Manager, mc metric.Collector, cloud aws.CloudAPI) error {
	nlbReconciler, err := newReconciler(config, mgr, mc, cloud)
	if err != nil {
		return err
	}
	nlbController, err := controller.New("nlb-service-controller", mgr, controller.Options{Reconciler: nlbReconciler})
	if err != nil {
		return err
	}
	if err := watchClusterEvents(nlbController, mgr.GetCache(), config.NLBServiceClass); err != nil {
		return err
	}

	return nil
}

func newReconciler(config *config.Configuration, mgr manager.Manager, mc metric.Collector, cloud aws.CloudAPI) (reconcile.Reconciler, error) {
	store, err := store.New(mgr, config)
	if err != nil {
		return nil, err
	}
	nameTagGenerator := generator.NewNameTagGenerator(*config)
	tagsController := tags.NewController(cloud)
	endpointResolver := backend.NewEndpointResolver(store, cloud)
	tgGroupController := tg.NewGroupController(cloud, store, nameTagGenerator, tagsController, endpointResolver)
	lsGroupController := ls.NewGroupController(store, cloud)
	lbController := lb.NewController(cloud, store,
		nameTagGenerator, tgGroupController, lsGroupController, tagsController)

	return &Reconciler{
		client:          mgr.GetClient(),
		cache:           mgr.GetCache(),
		recorder:        mgr.GetRecorder("nlb-service-controller"),
		store:           store,
		lbController:    lbController,
		metricCollector: mc,
	}, nil
}

func watchClusterEvents(c controller.Controller, cache cache.Cache, serviceClass string) error {
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handlers.EnqueueRequestsForServiceEvent{
		ServiceClass: serviceClass,
		Cache:        cache,
	}); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Endpoints{}}, &handlers.EnqueueRequestsForEndpointsEvent{
		ServiceClass: serviceClass,
		Cache:        cache,
	}); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Node{}}, &handlers.EnqueueRequestsForNodeEvent{
		ServiceClass: serviceClass,
		Cache:        cache,
	}); err != nil {
		return err
	}
	return nil
}
