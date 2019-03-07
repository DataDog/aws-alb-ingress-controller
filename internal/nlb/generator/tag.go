package generator

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
)

// Standard tag key names
const (
	TagKeyNamespace   = "kubernetes.io/namespace"
	TagKeyServiceName = "kubernetes.io/service-name"
	TagKeyServicePort = "kubernetes.io/service-port"
)

var _ tg.TagGenerator = (*TagGenerator)(nil)
var _ lb.TagGenerator = (*TagGenerator)(nil)

type TagGenerator struct {
	ClusterName string
	DefaultTags map[string]string
}

func (gen *TagGenerator) TagLB(namespace string, serviceName string) map[string]string {
	return gen.tagServiceResources(namespace, serviceName)
}

func (gen *TagGenerator) TagTGGroup(namespace string, serviceName string) map[string]string {
	return gen.tagServiceResources(namespace, serviceName)
}

func (gen *TagGenerator) TagTG(serviceName string, servicePort string) map[string]string {
	return map[string]string{
		TagKeyServiceName: serviceName,
		TagKeyServicePort: servicePort,
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
	return m
}
