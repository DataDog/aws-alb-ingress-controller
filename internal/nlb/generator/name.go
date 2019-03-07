package generator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
)

var _ tg.NameGenerator = (*NameGenerator)(nil)
var _ lb.NameGenerator = (*NameGenerator)(nil)

type NameGenerator struct {
	NLBNamePrefix string
}

func (gen *NameGenerator) NameLB(namespace string, serviceName string) string {
	hasher := md5.New()
	_, _ = hasher.Write([]byte(namespace + serviceName))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]

	r, _ := regexp.Compile("[[:^alnum:]]")
	name := fmt.Sprintf("%s-%s-%s",
		r.ReplaceAllString(gen.NLBNamePrefix, "-"),
		r.ReplaceAllString(namespace, ""),
		r.ReplaceAllString(serviceName, ""),
	)
	if len(name) > 26 {
		name = name[:26]
	}
	name = name + "-" + hash
	return name
}

func (gen *NameGenerator) NameTG(namespace string, serviceName, servicePort string, targetType string, protocol string) string {
	LBName := gen.NameLB(namespace, serviceName)

	hasher := md5.New()
	_, _ = hasher.Write([]byte(LBName))
	_, _ = hasher.Write([]byte(serviceName))
	_, _ = hasher.Write([]byte(servicePort))
	_, _ = hasher.Write([]byte(protocol))
	_, _ = hasher.Write([]byte(targetType))

	return fmt.Sprintf("%.12s-%.19s", gen.NLBNamePrefix, hex.EncodeToString(hasher.Sum(nil)))
}
