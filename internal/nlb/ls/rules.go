package ls

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/service/elbv2"
	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations/action"
)

// buildActions will build listener rule actions for specific authCfg and backend
func buildActions(ctx context.Context, serviceAnnos *annotations.Service, backend extensions.IngressBackend, tgGroup tg.TargetGroupGroup) ([]*elbv2.Action, error) {
	var actions []*elbv2.Action

	// Handle backend actions
	if action.Use(backend.ServicePort.String()) {
		// backend is based on annotation
		backendAction, err := serviceAnnos.Action.GetAction(backend.ServiceName)
		if err != nil {
			return nil, err
		}
		actions = append(actions, &backendAction)
	} else {
		// backend is based on service
		targetGroup, ok := tgGroup.TGByBackend[backend]
		if !ok {
			return nil, fmt.Errorf("unable to find targetGroup for backend %v:%v",
				backend.ServiceName, backend.ServicePort.String())
		}
		backendAction := elbv2.Action{
			Type:           aws.String(elbv2.ActionTypeEnumForward),
			TargetGroupArn: aws.String(targetGroup.Arn),
			ForwardConfig: &elbv2.ForwardActionConfig{
				TargetGroups: []*elbv2.TargetGroupTuple{
					{
						TargetGroupArn: aws.String(targetGroup.Arn),
					},
				},
			},
		}
		actions = append(actions, &backendAction)
	}

	for index, action := range actions {
		action.Order = aws.Int64(int64(index) + 1)
	}
	return actions, nil
}

func sortActions(actions []*elbv2.Action) {
	sort.Slice(actions, func(i, j int) bool {
		return aws.Int64Value(actions[i].Order) < aws.Int64Value(actions[j].Order)
	})
}
