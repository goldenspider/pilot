package etcd

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"istio.io/istio/pilot/pkg/model"
	pb "pilot/pkg/proto/etcd"
)

type ClusterManager struct {
	*zap.SugaredLogger
	*Client
	ds *DataSource
	nd *NodeManager
	sv *ServiceManager
	ep *InstanceManager
}

func NewClusterManager(l *zap.SugaredLogger, client *Client, ds *DataSource, nd *NodeManager, sv *ServiceManager, ep *InstanceManager) *ClusterManager {
	return &ClusterManager{SugaredLogger: l.Named("ClusterManager"), Client: client, ds: ds, nd: nd, sv: sv, ep: ep}
}

func (m *ClusterManager) initRouter(r *gin.RouterGroup) error {
	s := &pb.Cluster{}

	r.GET("/clusters", func(c *gin.Context) {
		instances, e := m.ds.List("/clusters/", s)
		if e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, instances)
	})

	r.GET("/clusters/:id", func(c *gin.Context) {
		id := c.Params.ByName("id")

		cluster, e := m.Get(id)
		if e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, cluster)
	})

	r.PUT("/clusters/:id", func(c *gin.Context) {
		id := c.Params.ByName("id")

		cluster := &pb.Cluster{}
		c.BindJSON(cluster)

		if id != cluster.Id {
			AbortWithError(m.SugaredLogger, c, fmt.Errorf("写入Cluster的ID（%s）和Path中的ID（%s）不一致。", cluster.Id, id))
			return
		}

		if e := m.Put(cluster); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}

		c.JSON(200, struct{}{})
	})

	r.DELETE("/clusters/:id", func(c *gin.Context) {
		id := c.Params.ByName("id")
		clusterKey := fmt.Sprintf("/clusters/%s", id)

		cluster, e := m.Get(id)
		if e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}

		for _, instId := range cluster.InstanceIds {
			inst := &pb.Instance{}
			instKey := fmt.Sprintf("/instances/%s", instId)
			if e := m.ds.Get(instKey, inst); e != nil {
				m.Errorf("查询Instance(%s)失败.%s", instId, e)
				AbortWithError(m.SugaredLogger, c, e)
				return
			}
			m.deleteServiceInstance(cluster, inst)
		}

		if e := m.ds.Delete(clusterKey); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	r.PUT("/clusters/:id/services/:service", func(c *gin.Context) {
		id := c.Params.ByName("id")
		sid := c.Params.ByName("service")

		sc := &pb.Service{}
		c.BindJSON(sc)

		if sid != sc.Id {
			AbortWithError(m.SugaredLogger, c, fmt.Errorf("写入Service的ID（%s）和Path中的ID（%s）不一致。",
				sc.Id, sid))
			return
		}

		if e := m.PutService(id, sc); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	r.DELETE("/clusters/:id/services/:service", func(c *gin.Context) {
		id := c.Params.ByName("id")
		sid := c.Params.ByName("service")

		if e := m.DeleteService(id, sid); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	r.GET("/clusters/:id/services/:service/instances", func(c *gin.Context) {
		id := c.Params.ByName("id")
		sid := c.Params.ByName("service")

		cluster, e := m.Get(id)
		if e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}

		if cluster.ServiceId != sid {
			AbortWithError(m.SugaredLogger, c, fmt.Errorf("写入Service的ID（%s）和Cluster中的ID（%s）不一致。",
				sid, cluster.ServiceId))
			return
		}

		insts, e := m.ep.GetInstances(sid)
		if e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}

		var instances []*pb.Instance
		for _, inst := range insts {
			ep := m.ep.FindInstanceByServiceInstance(inst)
			if ep != nil {
				instances = append(instances, ep)
			}
		}

		c.JSON(200, instances)
	})

	r.PUT("/clusters/:id/services/:service/instances", func(c *gin.Context) {
		id := c.Params.ByName("id")
		sid := c.Params.ByName("service")

		insts := []*pb.Instance{}
		c.BindJSON(insts)

		if e := m.PutServiceInstance(id, sid, insts); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	r.DELETE("/clusters/:id/services/:service/instances/:instance", func(c *gin.Context) {
		id := c.Params.ByName("id")
		//sid := c.Params.ByName("service")
		instId := c.Params.ByName("instance")

		if e := m.DeleteInstance(id, instId); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	return nil
}

func (m *ClusterManager) Get(id string) (*pb.Cluster, error) {
	path := fmt.Sprintf("/clusters/%s", id)

	cluster := &pb.Cluster{}
	if e := m.ds.Get(path, cluster); e != nil {
		return nil, errors.Wrapf(e, "查询Cluster(%s)失败", id)
	}
	return cluster, nil
}

func (m *ClusterManager) Put(cluster *pb.Cluster) error {
	path := fmt.Sprintf("/clusters/%s", cluster.Id)

	if e := m.ds.Put(path, cluster); e != nil {
		return errors.Wrapf(e, "写入Cluster(%s)失败", cluster.Id)
	}
	return nil
}

func (m *ClusterManager) Del(cluster *pb.Cluster) error {
	path := fmt.Sprintf("/clusters/%s", cluster.Id)

	if e := m.ds.Delete(path); e != nil {
		return errors.Wrapf(e, "删除Cluster(%s)失败", cluster.Id)
	}
	return nil
}

func (m *ClusterManager) buildServiceInstance(cluster *pb.Cluster, inst *pb.Instance) (*model.ServiceInstance, error) {
	service := &pb.Service{}
	serviceKey := fmt.Sprintf("/services/%s", inst.ServiceId)
	if e := m.ds.Get(serviceKey, service); e != nil {
		return nil, errors.Wrapf(e, "查询Service(%s)失败", inst.ServiceId)
	}

	svc := ConvertService(service)

	node := &pb.Node{}
	nodeKey := fmt.Sprintf("/nodes/%s", inst.NodeId)
	if e := m.ds.Get(nodeKey, node); e != nil {
		return nil, errors.Wrapf(e, "查询Node(%s)失败", inst.NodeId)
	}

	svcPortEntry, exists := svc.Ports.Get(inst.Port.Name)
	if !exists {
		return nil, fmt.Errorf("查询Service Proto(%s)失败", inst.Port.Name)
	}

	instance := &model.ServiceInstance{
		Endpoint: model.NetworkEndpoint{
			Address:     node.Ip,
			Port:        int(inst.Port.Port),
			ServicePort: svcPortEntry,
		},
		Service: svc,
		Labels: model.Labels{
			"cluster": cluster.Id,
		},
		AvailabilityZone: node.Az,
	}

	for k, v := range cluster.Labels {
		instance.Labels[k] = v
	}

	for k, v := range inst.Labels {
		instance.Labels[k] = v
	}
	fmt.Printf("buildServiceInstance ok:%+v\n", *instance)
	return instance, nil
}

func (m *ClusterManager) putServiceInstance(cluster *pb.Cluster, inst *pb.Instance) error {
	instance, e := m.buildServiceInstance(cluster, inst)
	if e != nil {
		return e
	}

	instKey := fmt.Sprintf("/instances/%s", inst.Id)
	if e := m.ds.Put(instKey, inst); e != nil {
		return errors.Wrapf(e, "写入Instance(%s)失败", inst.Id)
	}

	if e := m.ep.PutInstance(instance); e != nil {
		return errors.Wrapf(e, "写入pilot中instance失败. cluster = %s, service = %s, node=%s",
			cluster.Id, instance.Service.Hostname, instance.Endpoint.Address)
	}

	return nil
}

func (m *ClusterManager) deleteServiceInstance(cluster *pb.Cluster, inst *pb.Instance) error {
	instance, e := m.buildServiceInstance(cluster, inst)
	if e != nil {
		return e
	}

	if e := m.ep.DeleteInstance(string(instance.Service.Hostname), instance.Endpoint); e != nil {
		return errors.Wrapf(e, "删除pilot中instance失败. cluster = %s, service = %s, node=%s",
			cluster.Id, instance.Service.Hostname, instance.Endpoint.Address)
	}
	return nil
}

func (m *ClusterManager) PutServiceInstance(clusterId string, sid string, insts []*pb.Instance) error {
	cluster, e := m.Get(clusterId)
	if e != nil {
		return errors.Wrapf(e, "查询Cluster(%s)失败", clusterId)
	}

	if cluster.ServiceId != sid {
		return fmt.Errorf("查询Cluster ServiceId(%s)失败,sid(%s)", cluster.ServiceId, sid)
	}

	if e := m.syncServiceInstance(cluster, insts); e != nil {
		return e
	}

	for _, inst := range insts {
		if cluster.InstanceIds == nil {
			cluster.InstanceIds = make(map[string]string)
		}
		cluster.InstanceIds[inst.Id] = ""
	}

	return m.Put(cluster)
}

func (m *ClusterManager) PutService(clusterId string, sc *pb.Service) error {
	cluster, e := m.Get(clusterId)
	if e != nil {
		return errors.Wrapf(e, "查询Cluster(%s)失败", clusterId)
	}

	if sc.Id != string(ServiceHostname(sc.Name, sc.Namespace)) {
		sc.Id = string(ServiceHostname(sc.Name, sc.Namespace))
	}

	if cluster.ServiceId == sc.Id {
		return nil
	}

	if e := m.sv.PutService(sc); e != nil {
		return e
	}

	serviceKey := fmt.Sprintf("/services/%s", sc.Id)
	if e := m.ds.Put(serviceKey, sc); e != nil {
		return fmt.Errorf("failed to put service at %s. error = %v", serviceKey, e)
	}

	cluster.ServiceId = sc.Id
	return m.Put(cluster)
}

func (m *ClusterManager) DeleteService(clusterId string, serviceId string) error {
	cluster, e := m.Get(clusterId)
	if e != nil {
		return errors.Wrapf(e, "查询Cluster(%s)失败", clusterId)
	}

	if cluster.ServiceId != serviceId {
		return nil
	}

	for _, instid := range cluster.InstanceIds {
		inst := &pb.Instance{}
		instKey := fmt.Sprintf("/instances/%s", instid)
		if e := m.ds.Get(instKey, inst); e != nil {
			return errors.Wrapf(e, "Get Instance(%s)失败", instid)
		}

		if e := m.deleteServiceInstance(cluster, inst); e != nil {
			m.Errorf("failed to delete service instance, cluster = %s, service = %s, instId = %s, error = %v",
				cluster.Id, serviceId, inst.Id, e)
			continue
		}
	}

	cluster.ServiceId = ""
	cluster.InstanceIds = make(map[string]string)
	return m.Put(cluster)
}

func (m *ClusterManager) DeleteInstance(clusterId string, instanceId string) error {
	cluster, e := m.Get(clusterId)
	if e != nil {
		return errors.Wrapf(e, "查询Cluster(%s)失败", clusterId)
	}

	if instId, ok := cluster.InstanceIds[instanceId]; ok {
		inst := &pb.Instance{}
		instKey := fmt.Sprintf("/instances/%s", instId)
		if e := m.ds.Get(instKey, inst); e != nil {
			return errors.Wrapf(e, "Get Instance(%s)失败", instId)
		}

		if e := m.deleteServiceInstance(cluster, inst); e != nil {
			return errors.Wrapf(e, "failed to delete service instance, cluster = %s, service = %s, instId = %s, error = %v",
				cluster.Id, inst.ServiceId, inst.Id)
		}
	}

	delete(cluster.InstanceIds, instanceId)
	return m.Put(cluster)
}

func (m *ClusterManager) syncServiceInstance(cluster *pb.Cluster, insts []*pb.Instance) error {
	instances, e := m.Client.GetInstances(cluster.ServiceId)
	if e != nil {
		return errors.Wrapf(e, "查询pilot中service-%s的instance失败. cluster = %s, service = %s", cluster.Id, cluster.ServiceId)
	}

	idx := make(map[string]*model.ServiceInstance, len(instances))
	for _, instance := range instances {
		if instance.Labels["cluster"] == cluster.Id {
			idx[instance.Endpoint.Address] = instance
		}
	}

	for _, inst := range insts {
		if inst.ServiceId != cluster.ServiceId {
			continue
		}

		node := m.nd.GetNode(inst.NodeId)
		if node == nil {
			continue
		}

		if _, ok := idx[node.Ip]; ok {
			delete(idx, node.Ip)
		} else {
			if e := m.putServiceInstance(cluster, inst); e != nil {
				m.Errorf("failed to add new service instance, cluster = %s, service = %s, node = %s, error = %v",
					cluster.Id, inst.ServiceId, node, e)
				continue
			}
		}
	}

	// remove deleted instances
	for _, instance := range idx {
		if e := m.ep.DeleteInstance(cluster.ServiceId, instance.Endpoint); e != nil {
			m.Errorf("failed to delete service instance, cluster = %s, service = %s, node = %s, error = %v",
				instance.Labels["cluster"], instance.Service, instance.Endpoint.Address, e)
			continue
		}
	}

	return nil
}
