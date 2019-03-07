/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metric

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/metric/collectors"
)

// Collector defines the interface for a metric collector
type Collector interface {
	IncReconcileCount()
	IncReconcileErrorCount(string)
	SetManagedServices(map[string]int)

	IncAPIRequestCount(prometheus.Labels)
	IncAPIErrorCount(prometheus.Labels)
	IncAPIRetryCount(prometheus.Labels)

	RemoveMetrics(string)

	Start()
	Stop()
}

type collector struct {
	serviceController *collectors.Controller
	awsAPIController  *collectors.AWSAPIController

	registry *prometheus.Registry
}

// NewCollector creates a new metric collector the for service controller
func NewCollector(registry *prometheus.Registry, serviceClass string) (Collector, error) {
	ic := collectors.NewController(serviceClass)
	ac := collectors.NewAWSAPIController()

	return Collector(&collector{
		serviceController: ic,
		awsAPIController:  ac,
		registry:          registry,
	}), nil
}

func (c *collector) IncReconcileCount() {
	c.serviceController.IncReconcileCount()
}

func (c *collector) IncReconcileErrorCount(s string) {
	c.serviceController.IncReconcileErrorCount(s)
}

func (c *collector) SetManagedServices(i map[string]int) {
	c.serviceController.SetManagedServices(i, c.registry)
}

func (c *collector) IncAPIRequestCount(l prometheus.Labels) {
	c.awsAPIController.IncAPIRequestCount(l)
}

func (c *collector) IncAPIErrorCount(l prometheus.Labels) {
	c.awsAPIController.IncAPIErrorCount(l)
}

func (c *collector) IncAPIRetryCount(l prometheus.Labels) {
	c.awsAPIController.IncAPIRetryCount(l)
}

func (c *collector) RemoveMetrics(serviceName string) {
	c.serviceController.RemoveMetrics(serviceName)
}

func (c *collector) Start() {
	c.registry.MustRegister(c.serviceController)
	c.registry.MustRegister(c.awsAPIController)
}

func (c *collector) Stop() {
	c.registry.Unregister(c.serviceController)
	c.registry.Unregister(c.awsAPIController)
}
