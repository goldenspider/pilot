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
	ep *m.InstanceManager
	nd *m.NodeManager
	cs *m.ClusterManager
}

func NewManager(l *zap.SugaredLogger, cli *clientv3.Client, sv *m.ServiceManager, ds *m.DataSource, nd *m.NodeManager, ep *m.InstanceManager, cs *m.ClusterManager) *manager {
	return &manager{
		Client: m.NewClient(l.Named("manager"), cli, "/pilot", "/ns"),
		sv:     sv,
		ds:     ds,
		nd:     nd,
		ep:     ep,
		cs:     cs,
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
		Id: "192-168-170-140",
		Ip: "192.168.170.140",
		Az: "sh02",
	},
	&pb.Node{
		Id: "192-168-170-1",
		Ip: "192.168.170.1",
		Az: "sh02",
	},
}

var Endpoints []*pb.Instance = []*pb.Instance{
	&pb.Instance{
		ServiceId:      "hello_server.ns-a",
		ServiceVersion: "v1",
		NodeId:         "192-168-170-140",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50051,
			Protocol: string(convertProtocol("grpc")),
		},
		Labels: convertLabels([]string{"version|v1"}),
	},
	&pb.Instance{
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
	&pb.Instance{
		ServiceId:      "hello_server_alpha.ns-a",
		ServiceVersion: "v1",
		NodeId:         "192-168-170-140",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50052,
			Protocol: string(convertProtocol("grpc")),
		},
		Labels: convertLabels([]string{"version|v1"}),
	},
}

// type Cluster struct {
//	Id                   string
//	ServiceId            string
//	InstanceIds          map[string]string
//	Labels               map[string]string
//}

var Clusters []*pb.Cluster = []*pb.Cluster{
	&pb.Cluster{
		Id: "hello_s1",
		//ServiceId:   "hello_server.ns-a",
		//InstanceIds: map[string]string{m.InstaceId(Endpoints[0]): "", m.InstaceId(Endpoints[1]): ""},
	},
	&pb.Cluster{
		Id: "hello_s2",
		//ServiceId:   "hello_server_alpha.ns-a",
		//InstanceIds: map[string]string{m.InstaceId(Endpoints[2]): ""},
	},
}

func convertLabels(labels []string) model.Labels {
	out := make(model.Labels, len(labels))
	for _, tag := range labels {
		vals := strings.Split(tag, "|")
		// Labels not of form "key|value" are ignored to avoid possible collisions
		if len(vals) > 1 {
			out[vals[0]] = vals[1]
		} else {
			fmt.Printf("Tag %v ignored since it is not of form key|value", tag)
		}
	}
	return out
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

	client := m.NewClient(l.Sugar(), cli, "/pilot", "/ns")

	ds := m.NewDataSource(l.Sugar(), cli)

	sv := m.NewServiceManager(l.Sugar(), ds, client)

	nd := m.NewNodeManager(l.Sugar(), ds)
	ep := m.NewInstanceManager(l.Sugar(), client, ds, nd)
	cs := m.NewClusterManager(l.Sugar(), client, ds, nd, sv, ep)

	sm := NewManager(l.Sugar(), cli, sv, ds, nd, ep, cs)

	//name = /ns/bfcheck/bfcheck-s1/172.28.217.219:8010
	sm.ds.Put("/ns/hello_server.ns-a/hello_s1/192.168.170.140:50051", "")
	sm.ds.Put("/ns/hello_server.ns-a/hello_s1/192.168.170.1:50051", "")
	sm.ds.Put("/ns/hello_server_alpha.ns-a/hello_s1/192.168.170.140:50052", "")

	for _, node := range Nodes {
		e := sm.nd.PutNode(node)
		if e != nil {
			fmt.Println(e)
		}
	}
	//////////////////
	sm.cs.Put(Clusters[0])

	e := sm.cs.PutService(Clusters[0].Id, Services[0])
	if e != nil {
		fmt.Println(e)
	}

	e = sm.cs.PutServiceInstance(Clusters[0].Id, "hello_server.ns-a", Endpoints)
	if e != nil {
		fmt.Println(e)
	}

	//////////////////
	sm.cs.Put(Clusters[1])

	e = sm.cs.PutService(Clusters[1].Id, Services[1])
	if e != nil {
		fmt.Println(e)
	}

	e = sm.cs.PutServiceInstance(Clusters[1].Id, "hello_server_alpha.ns-a", Endpoints)
	if e != nil {
		fmt.Println(e)
	}
}
