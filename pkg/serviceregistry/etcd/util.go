package etcd

import (
	"istio.io/istio/pilot/pkg/model"

	"fmt"
	pb "pilot/pkg/proto/etcd"
)

const ClusterIPNone = "None"

// for test
var Services []*pb.Service = []*pb.Service{
	&pb.Service{
		Name:      "hello_server",
		Namespace: "ns-a",
		Version:   map[string]string{"v1": "test1", "v2": "test2"},
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     15001,
				Protocol: string(convertProtocol("grpc")),
			},
		},
	},
	&pb.Service{
		Name:      "hello_server_alpha",
		Namespace: "ns-a",
		Version:   map[string]string{"v1": ""},
		Ports: []*pb.Port{
			{
				Name:     "grpc",
				Port:     15001,
				Protocol: string(convertProtocol("grpc")),
			},
		},
	},
}

var Nodes []*pb.Node = []*pb.Node{
	&pb.Node{
		Id: "192-168-170-139",
		Ip: "192.168.170.139",
		Az: "sh02",
	},
	&pb.Node{
		Id: "192-168-170-1",
		Ip: "192.168.170.1",
		Az: "sh02",
	},
}

var Endpoints []pb.Instance = []pb.Instance{
	{
		ServiceId:      "hello_server.ns-a",
		ServiceVersion: "v1",
		NodeId:         "192-168-170-139",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50051,
			Protocol: string(convertProtocol("grpc")),
		},
		Labels: convertLabels([]string{"version|v1"}),
	},
	{
		ServiceId:      "hello_server.ns-a",
		ServiceVersion: "v2",
		NodeId:         "192-168-170-1",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50051,
			Protocol: string(convertProtocol("grpc")),
		},
		Labels: convertLabels([]string{"version|v2"}),
	},
	{
		ServiceId:      "hello_server_alpha.ns-a",
		ServiceVersion: "v1",
		NodeId:         "192-168-170-139",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50052,
			Protocol: string(convertProtocol("grpc")),
		},
		Labels: convertLabels([]string{"version|v1"}),
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

func getNode(nodeId string) *pb.Node {
	for _, node := range Nodes {
		if node.Id == nodeId {
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
