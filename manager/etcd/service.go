package etcd

import (
	"context"
	"github.com/golang/glog"
	"go.uber.org/zap"
	pb "pilot/pkg/proto/etcd"
	"time"
)

type ServiceManager struct {
	*zap.SugaredLogger
	*Client
	ds *DataSource
}

func NewServiceManager(l *zap.SugaredLogger, ds *DataSource, client *Client) *ServiceManager {
	return &ServiceManager{SugaredLogger: l.Named("ServiceManager"), ds: ds, Client: client}
}

func (c *ServiceManager) PutService(service *pb.Service) error {
	glog.Infof("put service, service = %#v", service)
	//service.Id = string(ServiceHostname(service.Name, service.Namespace))
	obj := ConvertService(service)

	bytes, err := MarshalServiceToYAML(obj)
	if err != nil {
		glog.Errorf("failed to marshal service. error = %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	key := c.ServiceKey(string(obj.Hostname))
	if _, err = c.Put(ctx, key, string(bytes)); err != nil {
		glog.Errorf("failed to put service. key = %s, value = %s, error = %v", key, string(bytes), err)
		return err
	}
	glog.Infof("put service, key = %s, value = %s", key, string(bytes), obj)
	return nil
}

func (c *ServiceManager) ServiceId(name, namespace string) string {
	return string(ServiceHostname(name, namespace))
}

func (c *ServiceManager) DelService(service string) error {
	glog.Infof("delete service, service = %s", service)

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	key := c.ServiceKey(service)
	if _, err := c.Delete(ctx, key); err != nil {
		glog.Errorf("failed to delete service. key = %s, error = %v", key, err)
		return err
	}

	glog.Infof("deleted service, service = %s", service)
	return nil
}
