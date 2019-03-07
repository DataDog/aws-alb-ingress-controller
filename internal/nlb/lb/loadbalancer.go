package lb

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// LoadBalancerController manages loadBalancer for service objects (HTTP) or service objects (TCP)
type Controller interface {
	// Reconcile will make sure an LoadBalancer exists for specified service (HTTP) or service (TCP).
	Reconcile(ctx context.Context, service *corev1.Service) (*LoadBalancer, error)

	// Deletes will ensure no LoadBalancer exists for specified key.
	Delete(ctx context.Context, key types.NamespacedName) error
}

// TODO: security groups must be set on target group instances, and are not permitted on the NLB.
// https://docs.aws.amazon.com/elasticloadbalancing/latest/network/target-group-register-targets.html#target-security-groups
// https://aws.amazon.com/premiumsupport/knowledge-center/security-group-load-balancer/
func NewController(
	cloud aws.CloudAPI,
	store store.Storer,
	nameTagGen NameTagGenerator,
	tgGroupController tg.GroupController,
	lsGroupController ls.GroupController,
	tagsController tags.Controller) Controller {
	attrsController := NewAttributesController(cloud)

	return &defaultController{
		cloud:             cloud,
		store:             store,
		nameTagGen:        nameTagGen,
		tgGroupController: tgGroupController,
		lsGroupController: lsGroupController,
		tagsController:    tagsController,
		attrsController:   attrsController,
	}
}

type loadBalancerConfig struct {
	Name string
	Tags map[string]string

	Type          *string
	Scheme        *string
	IpAddressType *string
	Subnets       []string
}

type defaultController struct {
	cloud aws.CloudAPI
	store store.Storer

	nameTagGen        NameTagGenerator
	tgGroupController tg.GroupController
	lsGroupController ls.GroupController
	tagsController    tags.Controller
	attrsController   AttributesController
}

var _ Controller = (*defaultController)(nil)

func (controller *defaultController) Reconcile(ctx context.Context, service *corev1.Service) (*LoadBalancer, error) {
	serviceAnnos, err := controller.store.GetServiceAnnotations(k8s.MetaNamespaceKey(service))
	if err != nil {
		return nil, err
	}

	lbConfig, err := controller.buildLBConfig(ctx, service, serviceAnnos)
	if err != nil {
		return nil, fmt.Errorf("failed to build LoadBalancer configuration due to %v", err)
	}
	if err := controller.validateLBConfig(ctx, service, lbConfig); err != nil {
		return nil, err
	}

	instance, err := controller.ensureLBInstance(ctx, lbConfig)
	if err != nil {
		return nil, err
	}
	lbArn := aws.StringValue(instance.LoadBalancerArn)
	if err := controller.attrsController.Reconcile(ctx, lbArn, serviceAnnos.LoadBalancer.Attributes); err != nil {
		return nil, fmt.Errorf("failed to reconcile attributes of %v due to %v", lbArn, err)
	}

	tgGroup, err := controller.tgGroupController.Reconcile(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile targetGroups due to %v", err)
	}
	if err := controller.lsGroupController.Reconcile(ctx, lbArn, service, tgGroup); err != nil {
		return nil, fmt.Errorf("failed to reconcile listeners due to %v", err)
	}
	if err := controller.tgGroupController.GC(ctx, tgGroup); err != nil {
		return nil, fmt.Errorf("failed to GC targetGroups due to %v", err)
	}

	return &LoadBalancer{
		Arn:     lbArn,
		DNSName: aws.StringValue(instance.DNSName),
	}, nil
}

func (controller *defaultController) Delete(ctx context.Context, key types.NamespacedName) error {
	lbName := controller.nameTagGen.NameLB(key.Namespace, key.Name)
	instance, err := controller.cloud.GetLoadBalancerByName(ctx, lbName)
	if err != nil {
		return fmt.Errorf("failed to find existing LoadBalancer due to %v", err)
	}
	if instance != nil {
		if err = controller.lsGroupController.Delete(ctx, aws.StringValue(instance.LoadBalancerArn)); err != nil {
			return fmt.Errorf("failed to delete listeners due to %v", err)
		}
		if err = controller.tgGroupController.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to GC targetGroups due to %v", err)
		}

		albctx.GetLogger(ctx).Infof("deleting LoadBalancer %v", aws.StringValue(instance.LoadBalancerArn))
		if err = controller.cloud.DeleteLoadBalancerByArn(ctx, aws.StringValue(instance.LoadBalancerArn)); err != nil {
			return err
		}
	}

	return nil
}

func (controller *defaultController) ensureLBInstance(ctx context.Context, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	instance, err := controller.cloud.GetLoadBalancerByName(ctx, lbConfig.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find existing LoadBalancer due to %v", err)
	}
	if instance == nil {
		instance, err = controller.newLBInstance(ctx, lbConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create LoadBalancer due to %v", err)
		}
		return instance, nil
	}
	if controller.isLBInstanceNeedRecreation(ctx, instance, lbConfig) {
		instance, err = controller.recreateLBInstance(ctx, instance, lbConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to recreate LoadBalancer due to %v", err)
		}
		return instance, nil
	}
	if err := controller.reconcileLBInstance(ctx, instance, lbConfig); err != nil {
		return nil, err
	}
	return instance, nil
}

func (controller *defaultController) newLBInstance(ctx context.Context, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	albctx.GetLogger(ctx).Infof("creating LoadBalancer %v", lbConfig.Name)
	resp, err := controller.cloud.CreateLoadBalancerWithContext(ctx, &elbv2.CreateLoadBalancerInput{
		Name:          aws.String(lbConfig.Name),
		Type:          lbConfig.Type,
		Scheme:        lbConfig.Scheme,
		IpAddressType: lbConfig.IpAddressType,
		Subnets:       aws.StringSlice(lbConfig.Subnets),
		Tags:          tags.ConvertToELBV2(lbConfig.Tags),
	})
	if err != nil {
		albctx.GetLogger(ctx).Errorf("failed to create LoadBalancer %v due to %v", lbConfig.Name, err)
		albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", "failed to create LoadBalancer %v due to %v", lbConfig.Name, err)
		return nil, err
	}

	instance := resp.LoadBalancers[0]
	albctx.GetLogger(ctx).Infof("LoadBalancer %v created, ARN: %v", lbConfig.Name, aws.StringValue(instance.LoadBalancerArn))
	albctx.GetEventf(ctx)(corev1.EventTypeNormal, "CREATE", "LoadBalancer %v created, ARN: %v", lbConfig.Name, aws.StringValue(instance.LoadBalancerArn))
	return instance, nil
}

func (controller *defaultController) recreateLBInstance(ctx context.Context, existingInstance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	existingLBArn := aws.StringValue(existingInstance.LoadBalancerArn)
	albctx.GetLogger(ctx).Infof("deleting LoadBalancer %v for recreation", existingLBArn)
	if err := controller.cloud.DeleteLoadBalancerByArn(ctx, existingLBArn); err != nil {
		return nil, err
	}
	return controller.newLBInstance(ctx, lbConfig)
}

func (controller *defaultController) reconcileLBInstance(ctx context.Context, instance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) error {
	lbArn := aws.StringValue(instance.LoadBalancerArn)
	if !util.DeepEqual(instance.IpAddressType, lbConfig.IpAddressType) {
		albctx.GetLogger(ctx).Infof("modifying LoadBalancer %v due to IpAddressType change (%v => %v)", lbArn, aws.StringValue(instance.IpAddressType), aws.StringValue(lbConfig.IpAddressType))
		if _, err := controller.cloud.SetIpAddressTypeWithContext(ctx, &elbv2.SetIpAddressTypeInput{
			LoadBalancerArn: instance.LoadBalancerArn,
			IpAddressType:   lbConfig.IpAddressType,
		}); err != nil {
			albctx.GetEventf(ctx)(corev1.EventTypeNormal, "ERROR", "failed to modify IpAddressType of %v due to %v", lbArn, err)
			return fmt.Errorf("failed to modify IpAddressType of %v due to %v", lbArn, err)
		}
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "MODIFY", "IpAddressType of %v modified", lbArn)
	}

	desiredSubnets := sets.NewString(lbConfig.Subnets...)
	currentSubnets := sets.NewString(aws.StringValueSlice(util.AvailabilityZones(instance.AvailabilityZones).AsSubnets())...)
	if !currentSubnets.Equal(desiredSubnets) {
		albctx.GetLogger(ctx).Infof("modifying LoadBalancer %v due to Subnets change (%v => %v)", lbArn, currentSubnets.List(), desiredSubnets.List())
		if _, err := controller.cloud.SetSubnetsWithContext(ctx, &elbv2.SetSubnetsInput{
			LoadBalancerArn: instance.LoadBalancerArn,
			Subnets:         aws.StringSlice(lbConfig.Subnets),
		}); err != nil {
			albctx.GetEventf(ctx)(corev1.EventTypeNormal, "ERROR", "failed to modify Subnets of %v due to %v", lbArn, err)
			return fmt.Errorf("failed to modify Subnets of %v due to %v", lbArn, err)
		}
	}

	if err := controller.tagsController.ReconcileELB(ctx, lbArn, lbConfig.Tags); err != nil {
		return fmt.Errorf("failed to reconcile tags of %v due to %v", lbArn, err)
	}
	return nil
}

func (controller *defaultController) isLBInstanceNeedRecreation(ctx context.Context, instance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) bool {
	if !util.DeepEqual(instance.Scheme, lbConfig.Scheme) {
		albctx.GetLogger(ctx).Infof("LoadBalancer %s need recreation due to scheme changed(%s => %s)",
			lbConfig.Name, aws.StringValue(instance.Scheme), aws.StringValue(lbConfig.Scheme))
		return true
	}
	return false
}

func (controller *defaultController) buildLBConfig(ctx context.Context, service *corev1.Service, serviceAnnos *annotations.Service) (*loadBalancerConfig, error) {
	lbTags := controller.nameTagGen.TagLB(service.Namespace, service.Name)
	for k, v := range serviceAnnos.Tags.LoadBalancer {
		lbTags[k] = v
	}

	subnets, err := controller.resolveSubnets(ctx, aws.StringValue(serviceAnnos.LoadBalancer.Scheme), serviceAnnos.LoadBalancer.Subnets)
	if err != nil {
		return nil, err
	}

	return &loadBalancerConfig{
		Name: controller.nameTagGen.NameLB(service.Namespace, service.Name),
		Tags: lbTags,

		Type:          aws.String(elbv2.LoadBalancerTypeEnumNetwork),
		Scheme:        serviceAnnos.LoadBalancer.Scheme,
		IpAddressType: serviceAnnos.LoadBalancer.IPAddressType,
		Subnets:       subnets,
	}, nil
}

func (controller *defaultController) validateLBConfig(ctx context.Context, service *corev1.Service, lbConfig *loadBalancerConfig) error {
	controllerCfg := controller.store.GetConfig()
	if controllerCfg.RestrictScheme && aws.StringValue(lbConfig.Scheme) == elbv2.LoadBalancerSchemeEnumInternetFacing {
		whitelisted := false
		for _, name := range controllerCfg.InternetFacingServices[service.Namespace] {
			if name == service.Name {
				whitelisted = true
				break
			}
		}
		if !whitelisted {
			return fmt.Errorf("service %v/%v is not in internetFacing whitelist", service.Namespace, service.Name)
		}
	}

	return nil
}

func (controller *defaultController) resolveSubnets(ctx context.Context, scheme string, in []string) ([]string, error) {
	if len(in) == 0 {
		subnets, err := controller.clusterSubnets(ctx, scheme)
		return subnets, err

	}

	var names []string
	var subnets []string

	for _, subnet := range in {
		if strings.HasPrefix(subnet, "subnet-") {
			subnets = append(subnets, subnet)
			continue
		}
		names = append(names, subnet)
	}

	if len(names) > 0 {
		o, err := controller.cloud.GetSubnetsByNameOrID(ctx, names)
		if err != nil {
			return subnets, err
		}

		for _, subnet := range o {
			subnets = append(subnets, aws.StringValue(subnet.SubnetId))
		}
	}

	sort.Strings(subnets)
	if len(subnets) != len(in) {
		return subnets, fmt.Errorf("not all subnets were resolvable, (%v != %v)", strings.Join(in, ","), strings.Join(subnets, ","))
	}

	return subnets, nil
}

func (controller *defaultController) clusterSubnets(ctx context.Context, scheme string) ([]string, error) {
	var subnetIds []string
	var useableSubnets []*ec2.Subnet
	var out []string
	var key string

	if scheme == elbv2.LoadBalancerSchemeEnumInternal {
		key = aws.TagNameSubnetInternalELB
	} else if scheme == elbv2.LoadBalancerSchemeEnumInternetFacing {
		key = aws.TagNameSubnetPublicELB
	} else {
		return nil, fmt.Errorf("invalid scheme [%s]", scheme)
	}

	clusterSubnets, err := controller.cloud.GetClusterSubnets()
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS tags. Error: %s", err.Error())
	}

	for arn, subnetTags := range clusterSubnets {
		for _, tag := range subnetTags {
			if aws.StringValue(tag.Key) == key {
				p := strings.Split(arn, "/")
				subnetID := p[len(p)-1]
				subnetIds = append(subnetIds, subnetID)
			}
		}
	}

	o, err := controller.cloud.GetSubnetsByNameOrID(ctx, subnetIds)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch subnets due to %v", err)
	}

	for _, subnet := range o {
		if subnetIsUsable(subnet, useableSubnets) {
			useableSubnets = append(useableSubnets, subnet)
			out = append(out, aws.StringValue(subnet.SubnetId))
		}
	}

	if len(out) < 2 {
		return nil, fmt.Errorf("retrieval of subnets failed to resolve 2 qualified subnets. Subnets must "+
			"contain the %s/<cluster name> tag with a value of shared or owned and the %s tag signifying it should be used for ALBs "+
			"Additionally, there must be at least 2 subnets with unique availability zones as required by "+
			"ALBs. Either tag subnets to meet this requirement or use the subnets annotation on the "+
			"service resource to explicitly call out what subnets to use for ALB creation. The subnets "+
			"that did resolve were %v", aws.TagNameCluster, key,
			log.Prettify(out))
	}

	sort.Strings(out)
	return out, nil
}

// subnetIsUsable determines if the subnet shares the same availability zone as a subnet in the
// existing list. If it does, false is returned as you cannot have albs provisioned to 2 subnets in
// the same availability zone.
func subnetIsUsable(new *ec2.Subnet, existing []*ec2.Subnet) bool {
	for _, subnet := range existing {
		if *new.AvailabilityZone == *subnet.AvailabilityZone {
			return false
		}
	}
	return true
}
