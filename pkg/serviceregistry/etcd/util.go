package etcd

import (
	"istio.io/istio/pilot/pkg/model"

	"fmt"
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
var Services []*Service = []*Service{
	&Service{
		Name:      "hello_server",
		Namespace: "ns-a",
		Ports:     model.PortList{convertPort(15001, "grpc")},
	},
}

var Nodes []*Node = []*Node{
	&Node{
		NodeName: "192-168-170-137",
		Ip:       "192.168.170.137",
		Az:       "sh02",
		Labels:   map[string]string{"version": "v1"},
	},
	&Node{
		NodeName: "192-168-170-1",
		Ip:       "192.168.170.1",
		Az:       "sh02",
		Labels:   map[string]string{"version": "v2"},
	},
}

var Endpoints []Endpoint = []Endpoint{
	{
		Name:      "hello_server",
		Namespace: "ns-a",
		NodeName:  "192-168-170-137",
		Ports:     model.PortList{convertPort(50051, "grpc")},
	},
	{
		Name:      "hello_server",
		Namespace: "ns-a",
		NodeName:  "192-168-170-1",
		Ports:     model.PortList{convertPort(50051, "grpc")},
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

func convertService(svc *Service) *model.Service {
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
		ports = append(ports, convertPort(port.Port, port.Name))
	}

	return &model.Service{
		Hostname:     serviceHostname(svc.Name, svc.Namespace),
		Ports:        ports,
		Address:      addr,
		MeshExternal: meshExternal,
		Resolution:   resolution,
		CreationTime: svc.CreationTimestamp,
	}
}

func getNode(nodeName string) *Node {
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
