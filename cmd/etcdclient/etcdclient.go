package main

import (
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"go.uber.org/zap"
	"istio.io/istio/pilot/pkg/model"
	m "pilot/manager/etcd"
	pb "pilot/pkg/proto/etcd"
	"strings"
)

type manager struct {
	*m.Client
	ds *m.DataSource
	sv *m.ServiceManager
}

func NewManager(cli *clientv3.Client, sv *m.ServiceManager, ds *m.DataSource) *manager {
	return &manager{
		Client: m.NewClient(cli, "/pilot", "/ns"),
		sv:     sv,
		ds:     ds,
	}

}

func convertProtocol(name string) model.Protocol {
	protocol := model.ParseProtocol(name)
	if protocol == model.ProtocolUnsupported {
		fmt.Printf("unsupported protocol value: %s", name)
		return model.ProtocolTCP
	}
	return protocol
}

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
		Id: "192-168-170-138",
		Ip: "192.168.170.138",
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
		NodeId:         "192-168-170-138",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50051,
			Protocol: string(convertProtocol("grpc")),
		},
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
	},
	{
		ServiceId:      "hello_server_alpha.ns-a",
		ServiceVersion: "v1",
		NodeId:         "192-168-170-138",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50052,
			Protocol: string(convertProtocol("grpc")),
		},
	},
}

func main() {
	var cfg clientv3.Config
	cfg.Endpoints = strings.Split("127.0.0.1:2379", ",")
	cli, err := clientv3.New(cfg)
	if err != nil {
		fmt.Printf("failed to open a etcd client. %s", err)
		return
	}

	l, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(l)

	ds := m.NewDataSource(l.Sugar(), cli)

	sv := m.NewServiceManager(l.Sugar(), ds, m.NewClient(cli, "/pilot", "/ns"))

	sm := NewManager(cli, sv, ds)

	for _, svc := range Services {
		e := sm.sv.PutService(svc)
		if e != nil {
			fmt.Println(e)
		}
	}
}
