package etcd

import (
	"fmt"
	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v2"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/log"
	pb "pilot/pkg/proto/etcd"
	"strings"
)

func Unique(vs []string) []string {
	mvs := make(map[string]struct{})
	for _, s := range vs {
		mvs[s] = struct{}{}
	}

	var rvs []string
	for key := range mvs {
		rvs = append(rvs, key)
	}

	return rvs
}

func ConvertService(svc *pb.Service) *model.Service {
	resolution := model.ClientSideLB
	meshExternal := false

	if svc.MeshExternal == true {
		resolution = model.Passthrough
		meshExternal = true
	}

	ports := make([]*model.Port, 0, len(svc.Ports))
	for _, port := range svc.Ports {
		ports = append(ports, convertPort(int(port.Port), port.Name))
	}

	return &model.Service{
		Hostname:     serviceHostname(svc.Name, svc.Namespace),
		Ports:        ports,
		Address:      model.UnspecifiedIP,
		MeshExternal: meshExternal,
		Resolution:   resolution,
	}
}

// parseHostname extracts service name and namespace from the service hostname
func ParseHostname(hostname model.Hostname) (name, namespace string) {
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

func ServiceHostname(name, namespace string) model.Hostname {
	if namespace == "" {
		return model.Hostname(name)
	}

	return model.Hostname(fmt.Sprintf("%s.%s", name, namespace))
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

func convertPbPort(port int, name string) *pb.Port {
	pt := convertPort(port, name)
	return &pb.Port{
		Name:     pt.Name,
		Port:     uint32(pt.Port),
		Protocol: string(convertProtocol(pt.Name)),
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

func serviceHostname(name, namespace string) model.Hostname {
	if namespace == "" {
		return model.Hostname(name)
	}

	return model.Hostname(fmt.Sprintf("%s.%s", name, namespace))
}

func UnmarshalFromSpecYAML(id string, kind string, yaml string) (*model.Config, error) {
	schema, exists := model.IstioConfigTypes.GetByType(kind)
	if !exists {
		return nil, fmt.Errorf("unrecognized type %q", kind)
	}

	spec, err := schema.FromYAML(yaml)
	if err != nil {
		return nil, multierror.Prefix(err, "unmarshal spec yaml error:")
	}

	if err := schema.Validate("", "", spec); err != nil {
		return nil, multierror.Prefix(err, "validation error:")
	}

	return &model.Config{
		ConfigMeta: model.ConfigMeta{
			Type: schema.Type,
			Name: id,
		},
		Spec: spec,
	}, nil
}

func MarshalConfigToYAML(config *model.Config) ([]byte, error) {
	schema, exists := model.IstioConfigTypes.GetByType(config.Type)
	if !exists {
		return nil, fmt.Errorf("unrecognized type %q", config.Type)
	}

	if err := schema.Validate("", "", config.Spec); err != nil {
		return nil, multierror.Prefix(err, "validation error:")
	}

	obj, err := crd.ConvertConfig(schema, *config)
	if err != nil {
		return nil, err
	}

	bytes, err := yaml.Marshal(obj)
	if err != nil {
		return nil, multierror.Prefix(err, "marshal error:")
	}
	return bytes, nil
}

func UnmarshalConfigFromYAML(bytes []byte) (*model.Config, error) {
	//obj := ConfigObject{}
	//if err := yaml.Unmarshal(bytes, &obj); err != nil {
	//	return nil, fmt.Errorf("cannot parse proto message: %v", err)
	//}
	//
	//schema, exists := model.IstioConfigTypes.GetByType(crd.CamelCaseToKabobCase(obj.Metadata.Type))
	//if !exists {
	//	return nil, fmt.Errorf("unrecognized type %v, parsed = %v", obj.Metadata.Type, obj)
	//}
	//
	//config, err := ConvertObject(schema, &obj, "")
	//if err != nil {
	//	return nil, fmt.Errorf("cannot parse proto message: %v", err)
	//}
	//return config, nil
	return nil, nil
}

func MarshalServiceToYAML(obj *model.Service) ([]byte, error) {
	bytes, err := yaml.Marshal(obj)
	if err != nil {
		return nil, multierror.Prefix(err, "marshal error:")
	}
	return bytes, nil
}

func UnmarshalServiceFromYAML(bytes []byte) (*model.Service, error) {
	obj := &model.Service{}
	if err := yaml.Unmarshal(bytes, obj); err != nil {
		return nil, multierror.Prefix(err, "marshal error:")
	}
	for _, p := range obj.Ports {
		if 0 == p.Port {
			p.Port = 80 // enable hostname in virtual_hosts's domains
		}
	}
	return obj, nil
}

func MarshalServiceInstanceToYAML(obj *model.ServiceInstance) ([]byte, error) {
	bytes, err := yaml.Marshal(obj)
	if err != nil {
		return nil, multierror.Prefix(err, "marshal error:")
	}
	return bytes, nil
}

func UnmarshalServiceInstanceFromYAML(bytes []byte) (*model.ServiceInstance, error) {
	obj := &model.ServiceInstance{}
	if err := yaml.Unmarshal(bytes, obj); err != nil {
		return nil, multierror.Prefix(err, "marshal error:")
	}
	return obj, nil
}
