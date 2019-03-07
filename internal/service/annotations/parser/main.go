/*
Copyright 2015 The Kubernetes Authors.

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

package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
)

var (
	// AnnotationsPrefix defines the common prefix used in the nginx ingress controller
	AnnotationsPrefix = "nlb.service.kubernetes.io"
)

type AnnotationInterface interface {
	GetAnnotations() map[string]string
}

// ServiceAnnotation has a method to parse annotations located in Service
type ServiceAnnotation interface {
	Parse(svc AnnotationInterface) (interface{}, error)
}

type ingAnnotations map[string]string

func (a ingAnnotations) parseBool(name string) (*bool, error) {
	val, ok := a[name]
	if ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return nil, errors.NewInvalidAnnotationContent(name, val)
		}
		return &b, nil
	}
	return nil, errors.ErrMissingAnnotations
}

func (a ingAnnotations) parseString(name string) (*string, error) {
	val, ok := a[name]
	if ok {
		return &val, nil
	}
	return nil, errors.ErrMissingAnnotations
}

func (a ingAnnotations) parseInt64(name string) (*int64, error) {
	val, ok := a[name]
	if ok {
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil, errors.NewInvalidAnnotationContent(name, val)
		}
		return &i, nil
	}
	return nil, errors.ErrMissingAnnotations
}

func checkAnnotation(name string, ing AnnotationInterface) error {
	if ing == nil || len(ing.GetAnnotations()) == 0 {
		return errors.ErrMissingAnnotations
	}
	if name == "" {
		return errors.ErrInvalidAnnotationName
	}

	return nil
}

// GetBoolAnnotation extracts a boolean from an Ingress annotation
func GetBoolAnnotation(name string, ing AnnotationInterface) (*bool, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, ing)
	if err != nil {
		return nil, err
	}
	return ingAnnotations(ing.GetAnnotations()).parseBool(v)
}

// GetStringAnnotation extracts a string from an Ingress annotation
func GetStringAnnotation(name string, ing AnnotationInterface) (*string, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, ing)
	if err != nil {
		return nil, err
	}
	return ingAnnotations(ing.GetAnnotations()).parseString(v)
}

// GetStringSliceAnnotation extracts a comma separated string list from an Ingress annotation
func GetStringSliceAnnotation(name string, ing AnnotationInterface) (out []string) {
	v, err := GetStringAnnotation(name, ing)
	if err != nil {
		return out
	}

	parts := strings.Split(*v, ",")
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, p)
	}

	return out
}

// GetStringAnnotations extracts a set of string annotations from an Ingress annotation
func GetStringAnnotations(name string, ing AnnotationInterface) (map[string]string, error) {
	prefix := GetAnnotationWithPrefix(name + ".")
	annos := ingAnnotations(ing.GetAnnotations())

	result := make(map[string]string)
	for k, v := range annos {
		if strings.HasPrefix(k, prefix) {
			key := strings.TrimPrefix(k, prefix)
			result[key] = v
		}
	}

	if len(result) == 0 {
		return result, errors.ErrMissingAnnotations
	}

	return result, nil
}

// GetInt64Annotation extracts an int from an Ingress annotation
func GetInt64Annotation(name string, ing AnnotationInterface) (*int64, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, ing)
	if err != nil {
		return nil, err
	}
	return ingAnnotations(ing.GetAnnotations()).parseInt64(v)
}


// GetAnnotationWithPrefix returns the prefix of ingress annotations
func GetAnnotationWithPrefix(suffix string) string {
	return fmt.Sprintf("%v/%v", AnnotationsPrefix, suffix)
}

// MergeString replaces a with b if it is undefined or the default value d
func MergeString(a, b *string, d string) *string {
	if b == nil {
		return a
	}

	if a == nil {
		return b
	}

	if *a == d {
		return b
	}

	return a
}

// MergeInt64 replaces a with b if it is undefined or the default value d
func MergeInt64(a, b *int64, d int64) *int64 {
	if b == nil {
		return a
	}

	if a == nil {
		return b
	}

	if *a == d {
		return b
	}

	return a
}

// MergeBool replaces a with b if it is undefined or the default value d
func MergeBool(a, b *bool, d bool) *bool {
	if b == nil {
		return a
	}

	if a == nil {
		return b
	}

	if *a == d {
		return b
	}

	return a
}

type String interface {
	Merge(*String)
}
