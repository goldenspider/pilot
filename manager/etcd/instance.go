package etcd

import (
	"context"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"go.uber.org/zap"
	"istio.io/istio/pilot/pkg/model"
	"net"
	pb "pilot/pkg/proto/etcd"

	"strconv"
	"strings"
	"time"
)

type InstanceManager struct {
	*zap.SugaredLogger
	Prefix                string
	serviceDiscoverPrefix string
	ds                    *DataSource
	nd                    *NodeManager
	sv                    *ServiceManager
	*Client
}

func NewInstanceManager(client *Client, ds *DataSource) *InstanceManager {
	return &InstanceManager{Prefix: "/pilot",
		serviceDiscoverPrefix: "/ns",
		ds:     ds,
		Client: client,
	}
}

func (c *InstanceManager) InstaceId(inst *pb.Instance) string {
	return fmt.Sprintf("%s-%s-%s", inst.ServiceId, inst.ServiceVersion, inst.NodeId)
}

func (c *InstanceManager) PutInstance(obj *model.ServiceInstance) error {
	glog.Infof("put instance, instance = %#v", obj)

	bytes, err := MarshalServiceInstanceToYAML(obj)
	if err != nil {
		glog.Errorf("failed to marshal instance. error = %v", err)
		return err
	}

	key := c.instanceKey(string(obj.Service.Hostname), obj.Endpoint)
	if err = c.ds.Put(key, string(bytes)); err != nil {
		glog.Errorf("failed to put instance. key = %s, value = %s, error = %v", key, string(bytes), err)
		return err
	}
	glog.Infof("put instance, key = %s, value = %s", key, string(bytes))
	return nil
}

func (c *InstanceManager) DeleteInstance(service string, endpoint model.NetworkEndpoint) error {
	glog.Infof("delete instance, service = %s, endpoint = %#v", service, endpoint)

	key := c.instanceKey(service, endpoint)
	if err := c.ds.Delete(key); err != nil {
		glog.Errorf("failed to delete service. key = %s, error = %v", key, err)
		return err
	}
	glog.Infof("delete instance, key = %s", key)
	return nil
}

func (c *InstanceManager) FindInstanceByServiceInstance(svcinst *model.ServiceInstance) *pb.Instance {
	node := c.nd.FindNodeByIP(svcinst.Endpoint.Address)
	if node == nil {
		c.Error("no found node. ip:%s", svcinst.Endpoint.Address)
		return nil
	}
	version, ok := svcinst.Labels["version"]
	if !ok {
		version = ""
	}

	inst := &pb.Instance{
		ServiceId:      string(svcinst.Service.Hostname),
		ServiceVersion: version,
		NodeId:         node.Id,
	}

	inst.Id = c.InstaceId(inst)
	instKey := fmt.Sprintf("/instances/%s", inst.Id)
	if e := c.ds.Get(instKey, inst); e != nil {
		c.Errorf("查询Instance(%s)失败.%s", inst.Id, e)
		return nil
	}

	return inst
}

func (c *InstanceManager) GetInstances(hostname string) ([]*model.ServiceInstance, error) {
	glog.Infof("get instances, hostname = %s", hostname)

	svc, err := c.GetService(hostname)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	key := c.onlinePrefix(string(svc.Hostname))
	rsp, err := c.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		glog.Errorf("failed to get online. key = %s, error = %v", key, err)
		return nil, err
	}
	//name := fmt.Sprintf("%s/%s/%s", PrefixServiceDiscover, app.Service, app.Cluster)
	//addr := app.ServiceAddress()
	//name = /ns/bfcheck/bfcheck-s1/172.28.217.219:8010
	online := make(map[string]int, 10)
	for _, kv := range rsp.Kvs {
		_, cluster, endpoint, ok := c.splitOnlineKey(string(kv.Key))
		if ok {
			ip, port, e := net.SplitHostPort(endpoint)
			if e == nil {
				if p, e := strconv.Atoi(port); e == nil {
					online[endpoint] = p
					online[cluster+"/"+ip] = p
				}
			}
		}
	}
	glog.Infof("get instances, online = %v", online)

	instancePrefix := c.instancePrefix(string(svc.Hostname))
	instanceRsp, err := c.Get(ctx, instancePrefix, clientv3.WithPrefix())
	if err != nil {
		glog.Errorf("failed to get instances. key = %s, error = %v", instancePrefix, err)
		return nil, err
	}

	instances := make([]*model.ServiceInstance, 0, len(instanceRsp.Kvs))
	for _, kv := range instanceRsp.Kvs {
		instanceKey := string(kv.Key)
		obj, err := UnmarshalServiceInstanceFromYAML(kv.Value)
		if err != nil {
			glog.Errorf("failed to parse instance. key = %s, value = %s, error = %v",
				string(kv.Key), string(kv.Value), err)
			continue
		}

		// check online
		endpoint := net.JoinHostPort(obj.Endpoint.Address, strconv.Itoa(obj.Endpoint.Port))
		if port, ok := online[endpoint]; !ok {
			cluster := obj.Labels["cluster"]
			port, ok = online[cluster+"/"+obj.Endpoint.Address]
			if ok {
				// online, but port is different
				obj.Endpoint.Port = port
			} else {
				// skip not online instance
				continue
			}
		}

		glog.Infof("get instance, key = %s, value = %s, instance = %#v", instanceKey, string(kv.Value), obj)
		instances = append(instances, obj)
	}

	return instances, nil
}

func (c *InstanceManager) instancePrefix(service string) string {
	return fmt.Sprintf("%s/services/%s/instances/", c.Prefix, service)
}

func (c *InstanceManager) instanceKey(service string, endpoint model.NetworkEndpoint) string {
	return fmt.Sprintf("%s/services/%s/instances/%s:%d",
		c.Prefix, service, endpoint.Address, endpoint.Port)
}

func (c *InstanceManager) splitInstanceKey(key string) (string, string, bool) {
	path := strings.TrimPrefix(key, c.Prefix)
	parts := strings.Split(path, "/")
	if 5 == len(parts) && "services" == parts[1] && "instances" == parts[3] {
		return parts[2], parts[4], true
	} else {
		return "", "", false
	}
}

func (c *InstanceManager) onlineRoot() string {
	return fmt.Sprintf("%s/", c.serviceDiscoverPrefix)
}

func (c *InstanceManager) onlinePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", c.serviceDiscoverPrefix, service)
}

func (c *InstanceManager) splitOnlineKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.serviceDiscoverPrefix)
	parts := strings.Split(path, "/")
	if 4 == len(parts) {
		return parts[1], parts[2], parts[3], true
	} else {
		return "", "", "", false
	}
}
