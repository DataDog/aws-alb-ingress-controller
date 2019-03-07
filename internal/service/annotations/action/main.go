package action

import (
	"encoding/json"
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/service/resolver"
)

const UseActionAnnotation = "use-annotation"

type Config struct {
	Actions map[string]*elbv2.Action
}

type action struct {
	r resolver.Resolver
}

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.ServiceAnnotation {
	return action{r}
}

// Parse parses the annotations contained in the resource
func (a action) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	actions := make(map[string]*elbv2.Action)
	annos, err := parser.GetStringAnnotations("actions", ing)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	for serviceName, raw := range annos {
		var data *elbv2.Action
		err := json.Unmarshal([]byte(raw), &data)
		if err != nil {
			return nil, err
		}
		err = data.Validate()
		if err != nil {
			return nil, err
		}
		switch *data.Type {
		case "forward":
			if data.TargetGroupArn == nil {
				return nil, fmt.Errorf("%v is type forward but did not include a valid TargetGroupArn configuration", serviceName)
			}
		default:
			return nil, fmt.Errorf("an invalid action type %v was configured in %v", *data.Type, serviceName)
		}
		setDefaults(data)
		actions[serviceName] = data
	}

	return &Config{
		Actions: actions,
	}, nil
}

// GetAction returns the action named serviceName configured by an annotation
func (c *Config) GetAction(serviceName string) (elbv2.Action, error) {
	action, ok := c.Actions[serviceName]
	if !ok {
		return elbv2.Action{}, fmt.Errorf(
			"backend with `servicePort: %s` was configured with `serviceName: %v` but an action annotation for %v is not set",
			UseActionAnnotation, serviceName, serviceName)
	}
	return *action, nil
}

// Use returns true if the parameter requested an annotation configured action
func Use(s string) bool {
	return s == UseActionAnnotation
}

func setDefaults(d *elbv2.Action) *elbv2.Action {
	return d
}

func Dummy() *Config {
	return &Config{
		Actions: map[string]*elbv2.Action{
			"forward": setDefaults(&elbv2.Action{
				Type:           aws.String(elbv2.ActionTypeEnumForward),
				TargetGroupArn: aws.String("legacy-tg-arn"),
			}),
		},
	}
}
