package generator

import (
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
)

// Standard tag key names
const (
	TagKeyNamespace   = "kubernetes.io/namespace"
	TagKeyServiceName = "kubernetes.io/service-name"
	TagKeyServicePort = "kubernetes.io/service-port"

	// These are tags that the upstream aws-load-balancer-controller expects on NLBs and TargetGroups
	// Our aws-alb-ingress-controller fork adds these tags to ease the migration back to upstream
	// ref:
	// https://github.com/kubernetes-sigs/aws-load-balancer-controller/blob/a30c980164db2d9f54596ab971be8d98453c679d/pkg/deploy/tracking/provider.go#L11-L24
	TagKeyLBCServiceResource = "service.k8s.aws/resource"
	TagKeyLBCCluster         = "elbv2.k8s.aws/cluster"
	TagKeyLBCStack           = "service.k8s.aws/stack"
)

var _ tg.TagGenerator = (*TagGenerator)(nil)
var _ lb.TagGenerator = (*TagGenerator)(nil)

type TagGenerator struct {
	ClusterName string
	DefaultTags map[string]string
}

func (gen *TagGenerator) TagLB(namespace string, serviceName string) map[string]string {
	t := gen.tagServiceResources(namespace, serviceName)
	t[TagKeyLBCServiceResource] = "LoadBalancer"
	return t
}

func (gen *TagGenerator) TagTGGroup(namespace string, serviceName string) map[string]string {
	return gen.tagServiceResources(namespace, serviceName)
}

func (gen *TagGenerator) TagTG(namespace, serviceName string, servicePort string) map[string]string {
	return map[string]string{
		TagKeyServiceName:        serviceName,
		TagKeyServicePort:        servicePort,
		TagKeyLBCServiceResource: fmt.Sprintf("%s/%s:%d", namespace, serviceName, servicePort),
	}
}

func (gen *TagGenerator) tagServiceResources(namespace string, serviceName string) map[string]string {
	m := make(map[string]string)
	for label, value := range gen.DefaultTags {
		m[label] = value
	}
	m["kubernetes.io/cluster/"+gen.ClusterName] = "owned"
	m[TagKeyNamespace] = namespace
	m[TagKeyServiceName] = serviceName
	m[TagKeyLBCCluster] = gen.ClusterName
	m[TagKeyLBCStack] = fmt.Sprintf("%s/%s", namespace, serviceName)
	return m
}
