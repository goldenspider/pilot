package etcd

import (
	"istio.io/istio/pilot/pkg/model"

	"fmt"
	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v2"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	pb "pilot/pkg/proto/etcd"
	"time"
)

const ClusterIPNone = "None"

type (
	ServiceManager struct {
	}

	Service struct {
		Name              string
		Namespace         string
		ClusterIP         string
		MeshExternal      bool
		Ports             model.PortList
		CreationTimestamp time.Time
	}

	Node struct {
		NodeName string
		Ip       string
		Az       string
		Labels   map[string]string
	}

	Endpoint struct {
		Name      string //service name
		Namespace string //service Namespace
		NodeName  string
		Ports     []*model.Port
	}
)

// for test
var Services []*pb.Service = []*pb.Service{
	&pb.Service{
		Name:      "hello_server",
		Namespace: "ns-a",
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     15001,
				Protocol: string(convertProtocol("grpc")),
			},
		},
		CreationTimestamp: time.Now().Format("2006-01-02 15:04:05.999999999"),
	},
	&pb.Service{
		Name:      "hello_server_alpha",
		Namespace: "ns-a",
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     15001,
				Protocol: string(convertProtocol("grpc")),
			},
		},
		CreationTimestamp: time.Now().Format("2006-01-02 15:04:05.999999999"),
	},
}

var Nodes []*pb.Node = []*pb.Node{
	&pb.Node{
		NodeName: "192-168-170-138",
		Ip:       "192.168.170.138",
		Az:       "sh02",
		Labels:   map[string]string{"version": "v1"},
	},
	&pb.Node{
		NodeName: "192-168-170-1",
		Ip:       "192.168.170.1",
		Az:       "sh02",
		Labels:   map[string]string{"version": "v2"},
	},
}

var Endpoints []pb.Endpoint = []pb.Endpoint{
	{
		Name:      "hello_server",
		Namespace: "ns-a",
		NodeName:  "192-168-170-138",
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     50051,
				Protocol: string(convertProtocol("grpc")),
			},
		},
	},
	{
		Name:      "hello_server",
		Namespace: "ns-a",
		NodeName:  "192-168-170-1",
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     50051,
				Protocol: string(convertProtocol("grpc")),
			},
		},
	},
	{
		Name:      "hello_server_alpha",
		Namespace: "ns-a",
		NodeName:  "192-168-170-138",
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     50052,
				Protocol: string(convertProtocol("grpc")),
			},
		},
	},
}

var vsdata string = `kind: VirtualService
metadata:
  name: hello_server
  namespace: ns-a
spec:
  hosts:
  - hello_server
  http:
  - match:
    - uri:
        prefix: "/hello"
    route:
    - destination:
        host: hello_server
        subset: v1
      weight: 90
    - destination:
        host: hello_server
        subset: v2
      weight: 10
`

var drdata string = `kind: DestinationRule
metadata:
  name: hello_server
  namespace: ns-a
spec:
  host: hello_server
  trafficPolicy:
    loadBalancer:
      simple: ROUND_ROBIN
  subsets:
  - name: v1
    labels:
      version: v1
  - name: v2
    labels:
      version: v2
`

var vsdata_alpha string = `kind: VirtualService
metadata:
  name: hello_server_alpha
  namespace: ns-a
spec:
  hosts:
  - hello_server_alpha
  http:
  - match:
    - uri:
        prefix: "/hello"
    route:
    - destination:
        host: hello_server_alpha
        subset: v1
`

var drdata_alpha string = `kind: DestinationRule
metadata:
  name: hello_server_alpha
  namespace: ns-a
spec:
  host: hello_server_alpha
  trafficPolicy:
    loadBalancer:
      simple: ROUND_ROBIN
  subsets:
  - name: v1
    labels:
      version: v1
`

func ConvertService(svc *pb.Service) *model.Service {
	addr := model.UnspecifiedIP
	if svc.ClusterIP != "" && svc.ClusterIP != ClusterIPNone {
		addr = svc.ClusterIP
	}

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

	time, _ := time.Parse("2006-01-02 15:04:05.999999999", svc.CreationTimestamp)
	return &model.Service{
		Hostname:     serviceHostname(svc.Name, svc.Namespace),
		Ports:        ports,
		Address:      addr,
		MeshExternal: meshExternal,
		Resolution:   resolution,
		CreationTime: time,
	}
}

func getNode(nodeName string) *pb.Node {
	for _, node := range Nodes {
		if node.NodeName == nodeName {
			return node
		}
	}
	return nil
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

	obj, err := ConvertConfig(schema, *config)
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
	obj := ConfigObject{}
	if err := yaml.Unmarshal(bytes, &obj); err != nil {
		return nil, fmt.Errorf("cannot parse proto message: %v", err)
	}

	schema, exists := model.IstioConfigTypes.GetByType(crd.CamelCaseToKabobCase(obj.Metadata.Type))
	if !exists {
		return nil, fmt.Errorf("unrecognized type %v, parsed = %v", obj.Metadata.Type, obj)
	}

	config, err := ConvertObject(schema, &obj, "")
	if err != nil {
		return nil, fmt.Errorf("cannot parse proto message: %v", err)
	}
	return config, nil
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
