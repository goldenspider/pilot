// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"strings"

	"fmt"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/log"
)

const (
	protocolTagName = "protocol"
	externalTagName = "external"
)

type (
	ServiceObject struct {
		model.Service
	}

	ServiceInstanceObject struct {
		model.ServiceInstance
		Service string `json:"service,omitempty"`
	}

	ConfigObject struct {
		Metadata model.ConfigMeta       `json:"metadata"`
		Spec     map[string]interface{} `json:"spec"`
	}
)

// VirtualService describes v1alpha3 route rules
//VirtualService = ProtoSchema{
//Type:        "virtual-service",
//Plural:      "virtual-services",
//Group:       "networking",
//Version:     "v1alpha3",
//MessageName: "istio.networking.v1alpha3.VirtualService",
//Gogo:        true,
//Validate:    ValidateVirtualService,
//}

// ConvertObject converts an IstioObject k8s-style object to the
// internal configuration model.
func ConvertObject(schema model.ProtoSchema, object *ConfigObject, domain string) (*model.Config, error) {
	data, err := schema.FromJSONMap(object.Spec)
	if err != nil {
		return nil, err
	}

	if err := schema.Validate("", "", data); err != nil {
		return nil, fmt.Errorf("configuration is invalid: %v", err)
	}

	meta := object.Metadata
	return &model.Config{
		ConfigMeta: model.ConfigMeta{
			Type:              schema.Type,
			Group:             ResourceGroup(&schema),
			Version:           schema.Version,
			Name:              meta.Name,
			Namespace:         meta.Namespace,
			Domain:            meta.Domain,
			Labels:            meta.Labels,
			Annotations:       meta.Annotations,
			ResourceVersion:   meta.ResourceVersion,
			CreationTimestamp: meta.CreationTimestamp,
		},
		Spec: data,
	}, nil

}

// ConvertConfig translates Istio config to k8s config JSON
func ConvertConfig(schema model.ProtoSchema, config model.Config) (*ConfigObject, error) {
	spec, err := model.ToJSONMap(config.Spec)
	if err != nil {
		return nil, err
	}

	return &ConfigObject{
		Metadata: model.ConfigMeta{
			Type:              schema.Type,
			Group:             ResourceGroup(&schema),
			Version:           schema.Version,
			Name:              config.Name,
			Namespace:         config.Namespace,
			Domain:            config.Domain,
			Labels:            config.Labels,
			Annotations:       config.Annotations,
			ResourceVersion:   config.ResourceVersion,
			CreationTimestamp: config.CreationTimestamp,
		},
		Spec: spec,
	}, nil
}

// ResourceName converts "my-name" to "myname".
// This is needed by k8s API server as dashes prevent kubectl from accessing CRDs
func ResourceName(s string) string {
	return strings.Replace(s, "-", "", -1)
}

// ResourceGroup generates the k8s API group for each schema.
func ResourceGroup(schema *model.ProtoSchema) string {
	return schema.Group + model.IstioAPIGroupDomain
}

func ConvertLabels(labels []string) model.Labels {
	out := make(model.Labels, len(labels))
	for _, tag := range labels {
		vals := strings.Split(tag, "|")
		// Labels not of form "key|value" are ignored to avoid possible collisions
		if len(vals) > 1 {
			out[vals[0]] = vals[1]
		} else {
			log.Warnf("Tag %v ignored since it is not of form key|value", tag)
		}
	}
	return out
}

func convertPort(port int, name string) *model.Port {
	if name == "" {
		name = "http"
	}

	return &model.Port{
		Name:     name,
		Port:     port,
		Protocol: convertProtocol(name),
	}
}

func convertProtocol(name string) model.Protocol {
	protocol := model.ParseProtocol(name)
	if protocol == model.ProtocolUnsupported {
		log.Warnf("unsupported protocol value: %s", name)
		return model.ProtocolTCP
	}
	return protocol
}

// parseHostname extracts service name and namespace from the service hostname
func parseHostname(hostname model.Hostname) (name, namespace string) {
	parts := strings.Split(hostname.String(), ".")
	if len(parts) < 2 {
		name = parts[0]
		namespace = ""
		return
	}
	name = parts[0]
	namespace = parts[1]
	return
}

func KeyFunc(name, namespace string) string {
	if len(namespace) == 0 {
		return name
	}
	return namespace + "/" + name
}

// ParseInputs reads multiple documents from `kubectl` output and checks with
// the schema. It also returns the list of unrecognized kinds as the second
// response.
//
// NOTE: This function only decodes a subset of the complete k8s
// ObjectMeta as identified by the fields in model.ConfigMeta. This
// would typically only be a problem if a user dumps an configuration
// object with kubectl and then re-ingests it.

//json or yaml
func ParseInputs(inputs string) ([]model.Config, error) {
	cfgs, _, e := crd.ParseInputs(inputs)
	return cfgs, e
}

// ParseInputsWithoutValidation same as ParseInputs, but do not apply schema validation.
func ParseInputsWithoutValidation(inputs string) ([]model.Config, error) {
	cfgs, _, e := crd.ParseInputsWithoutValidation(inputs)
	return cfgs, e
}
