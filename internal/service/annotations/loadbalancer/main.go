/*
Copyright 2016 The Kubernetes Authors.

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

package loadbalancer

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/resolver"
)

type PortData struct {
	Port   int64
	Scheme string
}

type Config struct {
	Scheme        *string
	IPAddressType *string
	Type          *string

	Subnets    []string
	Attributes []*elbv2.LoadBalancerAttribute
}

type loadBalancer struct {
	r resolver.Resolver
}

const (
	DefaultIPAddressType = elbv2.IpAddressTypeIpv4
	DefaultScheme        = elbv2.LoadBalancerSchemeEnumInternal
	DefaultType          = elbv2.LoadBalancerTypeEnumNetwork
)

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.ServiceAnnotation {
	return loadBalancer{r}
}

// Parse parses the annotations contained in the resource
func (lb loadBalancer) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	ipAddressType, err := parser.GetStringAnnotation("ip-address-type", ing)
	if err != nil {
		ipAddressType = aws.String(DefaultIPAddressType)
	}

	if *ipAddressType != elbv2.IpAddressTypeIpv4 && *ipAddressType != elbv2.IpAddressTypeDualstack {
		return nil, errors.NewInvalidAnnotationContentReason(fmt.Sprintf("IP address type must be either `%v` or `%v`", elbv2.IpAddressTypeIpv4, elbv2.IpAddressTypeDualstack))
	}

	scheme, err := parser.GetStringAnnotation("scheme", ing)
	if err != nil {
		scheme = aws.String(DefaultScheme)
	}

	if *scheme != elbv2.LoadBalancerSchemeEnumInternal && *scheme != elbv2.LoadBalancerSchemeEnumInternetFacing {
		return nil, errors.NewInvalidAnnotationContentReason(fmt.Sprintf("LB scheme must be either `%v` or `%v`", elbv2.LoadBalancerSchemeEnumInternal, elbv2.LoadBalancerSchemeEnumInternetFacing))
	}

	lbType, err := parser.GetStringAnnotation("type", ing)
	if lbType == nil {
		lbType = aws.String(DefaultType)
	}

	attributes, err := parseAttributes(ing)
	if err != nil {
		return nil, err
	}

	subnets := parser.GetStringSliceAnnotation("subnets", ing)

	return &Config{
		Scheme:        scheme,
		IPAddressType: ipAddressType,
		Attributes:    attributes,
		Subnets:       subnets,
	}, nil
}

func parseAttributes(ing parser.AnnotationInterface) ([]*elbv2.LoadBalancerAttribute, error) {
	var badAttrs []string
	var lbattrs []*elbv2.LoadBalancerAttribute

	attrs := parser.GetStringSliceAnnotation("load-balancer-attributes", ing)
	oldattrs := parser.GetStringSliceAnnotation("attributes", ing)
	if len(attrs) == 0 && len(oldattrs) != 0 {
		attrs = oldattrs
	}

	if attrs == nil {
		return nil, nil
	}

	for _, attr := range attrs {
		parts := strings.Split(attr, "=")
		switch {
		case attr == "":
			continue
		case len(parts) != 2:
			badAttrs = append(badAttrs, attr)
			continue
		}
		lbattrs = append(lbattrs, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(strings.TrimSpace(parts[0])),
			Value: aws.String(strings.TrimSpace(parts[1])),
		})
	}

	if len(badAttrs) > 0 {
		return nil, fmt.Errorf("unable to parse `%s` into Key=Value pair(s)", strings.Join(badAttrs, ", "))
	}
	return lbattrs, nil
}

func Dummy() *Config {
	return &Config{
		Scheme:        aws.String(elbv2.LoadBalancerSchemeEnumInternal),
		IPAddressType: aws.String(elbv2.IpAddressTypeIpv4),
	}
}
