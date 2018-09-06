package main

import (
	"encoding/json"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"istio.io/istio/pilot/pkg/model"
	"net/http"
	"os"
	"os/signal"
	m "pilot/manager/etcd"
	pb "pilot/pkg/proto/etcd"
	"strings"
	"syscall"
)

type manager struct {
	*zap.SugaredLogger
	*m.Client
	Http *http.Server
	ds   *m.DataSource
	sv   *m.ServiceManager
	ep   *m.InstanceManager
	nd   *m.NodeManager
	cs   *m.ClusterManager
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
		Id: "192-168-170-141",
		Ip: "192.168.170.141",
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
		NodeId:         "192-168-170-141",
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
		NodeId:         "192-168-170-141",
		Port: &pb.Port{
			Name:     "grpc",
			Port:     50052,
			Protocol: string(convertProtocol("grpc")),
		},
		Labels: convertLabels([]string{"version|v1"}),
	},
}

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

func NewManager() *manager {
	var cfg clientv3.Config
	cfg.Endpoints = strings.Split("127.0.0.1:2379", ",")
	cli, err := clientv3.New(cfg)
	if err != nil {
		fmt.Printf("failed to open a etcd client. %s", err)
		return nil
	}

	l, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(l)

	client := m.NewClient(l.Sugar(), cli, "/pilot", "/ns")

	ds := m.NewDataSource(l.Sugar(), cli)

	sv := m.NewServiceManager(l.Sugar(), ds, client)

	nd := m.NewNodeManager(l.Sugar(), ds)
	ep := m.NewInstanceManager(l.Sugar(), client, ds, nd)
	cs := m.NewClusterManager(l.Sugar(), client, ds, nd, sv, ep)

	m := &manager{
		SugaredLogger: l.Sugar().Named("manager"),
		Client:        client,
		ds:            ds,
		sv:            sv,
		nd:            nd,
		ep:            ep,
		cs:            cs,
	}

	m.Http = &http.Server{}
	m.Http.Handler = m.initRouter()

	return m

}

func (m *manager) initRouter() http.Handler {
	r := gin.Default()
	r.Use(gin.ErrorLogger())
	s := r.Group("/etcd/api")

	if e := m.nd.InitRouter(s); e != nil {
		m.Panicf("failed to init router of nodes. err = %v", e)
	}

	if e := m.sv.InitRouter(s); e != nil {
		m.Panicf("failed to init router of services. err = %v", e)
	}

	if e := m.cs.InitRouter(s); e != nil {
		m.Panicf("failed to init router of sets. err = %v", e)
	}

	return r
}

func GetConfigJSON(cfg interface{}) string {
	bytes, e := json.MarshalIndent(cfg, "", "  ")
	if e != nil {
		panic(e)
	}
	return string(bytes)
}

func main() {
	exit := make(chan os.Signal, 10)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	m := NewManager()
	if m == nil {
		return
	}

	fmt.Println(GetConfigJSON(Nodes[0]))
	fmt.Println(GetConfigJSON(Nodes[1]))
	fmt.Println(GetConfigJSON(Clusters[0]))
	fmt.Println(GetConfigJSON(Clusters[1]))
	fmt.Println(GetConfigJSON(Services[0]))
	fmt.Println(GetConfigJSON(Services[1]))
	fmt.Println(GetConfigJSON(Endpoints[0]))
	fmt.Println(GetConfigJSON(Endpoints[1]))
	fmt.Println(GetConfigJSON(Endpoints[2]))
	/*
		{
		  "Id": "192-168-170-141",
		  "Ip": "192.168.170.141",
		  "Az": "sh02"
		}
		{
		  "Id": "192-168-170-1",
		  "Ip": "192.168.170.1",
		  "Az": "sh02"
		}
		{
		  "Id": "hello_s1"
		}
		{
		  "Id": "hello_s2"
		}
		{
		  "Name": "hello_server",
		  "Namespace": "ns-a",
		  "Version": {
		    "v1": "test1",
		    "v2": "test2"
		  },
		  "Ports": [
		    {
		      "Name": "grpc",
		      "Port": 15001,
		      "Protocol": "GRPC"
		    }
		  ]
		}
		{
		  "Name": "hello_server_alpha",
		  "Namespace": "ns-a",
		  "Version": {
		    "v1": ""
		  },
		  "Ports": [
		    {
		      "Name": "grpc",
		      "Port": 15001,
		      "Protocol": "GRPC"
		    }
		  ]
		}
		{
		  "ServiceId": "hello_server.ns-a",
		  "ServiceVersion": "v1",
		  "NodeId": "192-168-170-141",
		  "Port": {
		    "Name": "grpc",
		    "Port": 50051,
		    "Protocol": "GRPC"
		  },
		  "Labels": {
		    "version": "v1"
		  }
		}
		{
		  "ServiceId": "hello_server.ns-a",
		  "ServiceVersion": "v2",
		  "NodeId": "192-168-170-1",
		  "Port": {
		    "Name": "grpc",
		    "Port": 50051,
		    "Protocol": "GRPC"
		  },
		  "Labels": {
		    "version": "v2"
		  }
		}
		{
		  "ServiceId": "hello_server_alpha.ns-a",
		  "ServiceVersion": "v1",
		  "NodeId": "192-168-170-141",
		  "Port": {
		    "Name": "grpc",
		    "Port": 50052,
		    "Protocol": "GRPC"
		  },
		  "Labels": {
		    "version": "v1"
		  }
		}

	*/

	m.ds.Put("/ns/hello_server.ns-a/hello_s1/192.168.170.141:50051", "")
	m.ds.Put("/ns/hello_server.ns-a/hello_s1/192.168.170.1:50051", "")
	m.ds.Put("/ns/hello_server_alpha.ns-a/hello_s1/192.168.170.141:50052", "")

	//for _, node := range Nodes {
	//	e := m.nd.PutNode(node)
	//	if e != nil {
	//		fmt.Println(e)
	//	}
	//}
	////////////////////
	//m.cs.Put(Clusters[0])
	//
	//e := m.cs.PutService(Clusters[0].Id, Services[0])
	//if e != nil {
	//	fmt.Println(e)
	//}
	//
	//e = m.cs.PutServiceInstance(Clusters[0].Id, "hello_server.ns-a", Endpoints)
	//if e != nil {
	//	fmt.Println(e)
	//}
	//
	////////////////////
	//m.cs.Put(Clusters[1])
	//
	//e = m.cs.PutService(Clusters[1].Id, Services[1])
	//if e != nil {
	//	fmt.Println(e)
	//}
	//
	//e = m.cs.PutServiceInstance(Clusters[1].Id, "hello_server_alpha.ns-a", Endpoints)
	//if e != nil {
	//	fmt.Println(e)
	//}
	////////////////////
	m.Http.Addr = "0.0.0.0:8080"
	go func() {
		fmt.Printf("The HTTP server is listen at %s", m.Http.Addr)
		if e := m.Http.ListenAndServe(); e != nil {
			fmt.Errorf("failed to listen and serve http server. %s", zap.Error(e))
		}
		fmt.Println("The HTTP server is exited.")
	}()
	fmt.Println("HTTP server is started.")

	sig := <-exit
	m.Info("get signal.", zap.String("Signal", sig.String()))
}
