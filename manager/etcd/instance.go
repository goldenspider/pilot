package etcd

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"istio.io/istio/pilot/pkg/model"
	pb "pilot/pkg/proto/etcd"

	"github.com/pkg/errors"
	"strings"
	"time"
)

type InstanceManager struct {
	*zap.SugaredLogger
	ds *DataSource
	nd *NodeManager
	*Client
}

func NewInstanceManager(l *zap.SugaredLogger, client *Client, ds *DataSource, nd *NodeManager) *InstanceManager {
	return &InstanceManager{
		SugaredLogger: l.Named("InstanceManager"),
		ds:            ds,
		nd:            nd,
		Client:        client,
	}
}

func InstaceId(inst *pb.Instance) string {
	return fmt.Sprintf("%s-%s-%s", inst.ServiceId, inst.ServiceVersion, inst.NodeId)
}

func (c *InstanceManager) InstaceId(inst *pb.Instance) string {
	return fmt.Sprintf("%s-%s-%s", inst.ServiceId, inst.ServiceVersion, inst.NodeId)
}

func (c *InstanceManager) PutInstance(obj *model.ServiceInstance) error {
	c.Infof("put instance, instance = %#v", obj)

	bytes, err := MarshalServiceInstanceToYAML(obj)
	if err != nil {
		c.Errorf("failed to marshal instance. error = %v", err)
		return err
	}

	key := c.instanceKey(string(obj.Service.Hostname), obj.Endpoint)

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	if _, e := c.Client.Put(ctx, key, string(bytes)); e != nil {
		return errors.Wrapf(e, "failed to put instance. key = %s, value = %s, error = %v", key, string(bytes))
	}

	c.Infof("put instance, key = %s, value = %s", key, string(bytes))
	return nil
}

func (c *InstanceManager) DeleteInstance(service string, endpoint model.NetworkEndpoint) error {
	c.Infof("delete instance, service = %s, endpoint = %#v", service, endpoint)

	key := c.instanceKey(service, endpoint)
	if err := c.ds.Delete(key); err != nil {
		c.Errorf("failed to delete service. key = %s, error = %v", key, err)
		return err
	}
	c.Infof("delete instance, key = %s", key)
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
	return fmt.Sprintf("%s/", c.ServiceDiscoverPrefix)
}

func (c *InstanceManager) onlinePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", c.ServiceDiscoverPrefix, service)
}

func (c *InstanceManager) splitOnlineKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.ServiceDiscoverPrefix)
	parts := strings.Split(path, "/")
	if 4 == len(parts) {
		return parts[1], parts[2], parts[3], true
	} else {
		return "", "", "", false
	}
}
