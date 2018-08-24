package main

import (
	"context"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"istio.io/istio/pilot/pkg/model"
	pb "pilot/pkg/proto/etcd"
	"pilot/pkg/serviceregistry/etcd"
	"strings"
	"time"
)

type ServiceManager struct {
	prefix string

	*clientv3.Client
}

func NewServiceManager(cli *clientv3.Client) *ServiceManager {
	return &ServiceManager{Client: cli,
		prefix: "/pilot",
	}
}

func (c *ServiceManager) servicePrefix() string {
	return fmt.Sprintf("%s/services/", c.prefix)
}

func (c *ServiceManager) serviceKey(service string) string {
	return fmt.Sprintf("%s/services/%s", c.prefix, service)
}

func (c *ServiceManager) PutService(service *pb.Service) error {
	glog.Infof("put service, service = %#v", service)
	obj := etcd.ConvertService(service)

	bytes, err := etcd.MarshalServiceToYAML(obj)
	if err != nil {
		glog.Errorf("failed to marshal service. error = %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	key := c.serviceKey(string(obj.Hostname))
	if _, err = c.Put(ctx, key, string(bytes)); err != nil {
		glog.Errorf("failed to put service. key = %s, value = %s, error = %v", key, string(bytes), err)
		return err
	}
	glog.Infof("put service, key = %s, value = %s", key, string(bytes), obj)
	return nil
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

func main() {
	var cfg clientv3.Config
	cfg.Endpoints = strings.Split("127.0.0.1:2379", ",")
	cli, err := clientv3.New(cfg)
	if err != nil {
		fmt.Printf("failed to open a etcd client. %s", err)
		return
	}
	sm := NewServiceManager(cli)

	for _, svc := range Services {
		e := sm.PutService(svc)
		if e != nil {
			fmt.Println(e)
		}
	}
}
