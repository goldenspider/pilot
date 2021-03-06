// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"context"

	"fmt"
	etcd "github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"github.com/prometheus/common/log"
	"istio.io/istio/pilot/pkg/model"
	m "pilot/manager/etcd"
)

type InstanceHandler = func(*model.ServiceInstance, model.Event)
type ServiceHandler = func(*model.Service, model.Event)

type (
	// Controller communicates with Consul and monitors for changes
	Controller struct {
		client           *m.Client
		instanceHandlers []InstanceHandler
		serviceHandlers  []ServiceHandler
	}
)

// NewController instantiates a new Etcd controller
func NewController(client *m.Client) *Controller {
	return &Controller{
		client:           client,
		instanceHandlers: make([]InstanceHandler, 0),
		serviceHandlers:  make([]ServiceHandler, 0),
	}
}

// Services list declarations of all services in the system
func (c *Controller) Services() ([]*model.Service, error) {
	log.Info("Services")
	names, err := c.client.GetServiceNames()
	if err != nil {
		return nil, err
	}

	services := make([]*model.Service, 0, len(names))
	for _, name := range names {
		service, err := c.client.GetService(name)
		if err != nil {
			return nil, err
		}
		services = append(services, service)
	}
	return services, nil
}

// GetService retrieves a service by host name if it exists
func (c *Controller) GetService(hostname model.Hostname) (*model.Service, error) {
	return nil, nil
}

// GetServiceAttributes retrieves namespace of a service if it exists.
func (c *Controller) GetServiceAttributes(hostname model.Hostname) (*model.ServiceAttributes, error) {
	log.Infof("GetServiceAttributes hostname=%+v", hostname)
	return nil, nil
}

// ManagementPorts retrieves set of health check ports by instance IP.
// This does not apply to Consul service registry, as Consul does not
// manage the service instances. In future, when we integrate Nomad, we
// might revisit this function.
func (c *Controller) ManagementPorts(addr string) model.PortList {
	log.Infof("ManagementPorts addr=%s", addr)
	return nil
}

// WorkloadHealthCheckInfo retrieves set of health check info by instance IP.
// This does not apply to Consul service registry, as Consul does not
// manage the service instances. In future, when we integrate Nomad, we
// might revisit this function.
func (c *Controller) WorkloadHealthCheckInfo(addr string) model.ProbeList {
	return nil
}

// Instances retrieves instances for a service and its ports that match
// any of the supplied labels. All instances match an empty tag list.
func (c *Controller) Instances(hostname model.Hostname, ports []string,
	labels model.LabelsCollection) ([]*model.ServiceInstance, error) {
	return nil, fmt.Errorf("NOT IMPLEMENTED")
}

// InstancesByPort retrieves instances for a service that match
// any of the supplied labels. All instances match an empty tag list.
func (c *Controller) InstancesByPort(hostname model.Hostname, reqSvcPort int,
	labelsList model.LabelsCollection) ([]*model.ServiceInstance, error) {
	fmt.Printf("\nInstancesByPort hostname=%s reqSvcPort=%d labelsList=%s\n", hostname, reqSvcPort, labelsList)

	objs, err := c.client.GetInstances(string(hostname))
	if err != nil {
		return nil, err
	}

	var out []*model.ServiceInstance
	for _, obj := range objs {
		if !labelsList.HasSubsetOf(obj.Labels) {
			continue
		}

		if obj.Endpoint.ServicePort.Port == reqSvcPort {
			out = append(out, obj)
		}
	}

	return out, nil
}

// GetProxyServiceInstances lists service instances co-located with a given proxy
func (c *Controller) GetProxyServiceInstances(node *model.Proxy) ([]*model.ServiceInstance, error) {
	log.Infof("GetProxyServiceInstances node=%+v", node)
	return nil, nil
}

// Run all controllers until a signal is received
func (c *Controller) Run(stop <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	wch := c.client.Watch(ctx, c.client.ServicePrefix(), etcd.WithPrefix())
	och := c.client.Watch(ctx, c.client.OnlineRoot(), etcd.WithPrefix())

	glog.Info("controller is running...")
	for {
		select {
		case <-stop:
			cancel()
			return
		case wresp := <-wch:
			for _, event := range wresp.Events {
				k := string(event.Kv.Key)
				glog.Infof("received an event. type = %s, key = %s", event.Type.String(), k)

				if _, ok := c.client.SplitServiceKey(k); ok {
					switch event.Type {
					case etcd.EventTypePut:
						for _, h := range c.serviceHandlers {
							go h(&model.Service{}, model.EventAdd)
						}

					case etcd.EventTypeDelete:
						for _, h := range c.serviceHandlers {
							go h(&model.Service{}, model.EventDelete)
						}
					}
				}

				if _, _, ok := c.client.SplitInstanceKey(k); ok {
					switch event.Type {
					case etcd.EventTypePut:
						for _, h := range c.instanceHandlers {
							go h(&model.ServiceInstance{}, model.EventAdd)
						}

					case etcd.EventTypeDelete:
						for _, h := range c.instanceHandlers {
							go h(&model.ServiceInstance{}, model.EventDelete)
						}
					}
				}
			}
		case onlineResp := <-och:
			for _, event := range onlineResp.Events {
				k := string(event.Kv.Key)
				glog.Infof("received an event. type = %s, key = %s", event.Type.String(), k)

				if _, _, _, ok := c.client.SplitOnlineKey(k); ok {
					switch event.Type {
					case etcd.EventTypePut:
						for _, h := range c.instanceHandlers {
							go h(&model.ServiceInstance{}, model.EventAdd)
						}

					case etcd.EventTypeDelete:
						for _, h := range c.instanceHandlers {
							go h(&model.ServiceInstance{}, model.EventDelete)
						}
					}
				}
			}
		}
	}
	glog.Warning("controller is exit")
}

// AppendServiceHandler implements a service catalog operation
func (c *Controller) AppendServiceHandler(f ServiceHandler) error {
	c.serviceHandlers = append(c.serviceHandlers, f)
	return nil
}

// AppendInstanceHandler implements a service catalog operation
func (c *Controller) AppendInstanceHandler(f InstanceHandler) error {
	c.instanceHandlers = append(c.instanceHandlers, f)
	return nil
}

// GetIstioServiceAccounts implements model.ServiceAccounts operation TODO
func (c *Controller) GetIstioServiceAccounts(hostname model.Hostname, ports []string) []string {
	// Need to get service account of service registered with consul
	// Currently Consul does not have service account or equivalent concept
	// As a step-1, to enabling istio security in Consul, We assume all the services run in default service account
	// This will allow all the consul services to do mTLS
	// Follow - https://goo.gl/Dt11Ct

	return []string{
		"spiffe://cluster.local/ns/default/sa/default",
	}
}
