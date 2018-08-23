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
)

type InstanceHandler = func(*model.ServiceInstance, model.Event)
type ServiceHandler = func(*model.Service, model.Event)

type (
	// Controller communicates with Consul and monitors for changes
	Controller struct {
		client           *Client
		instanceHandlers []InstanceHandler
		serviceHandlers  []ServiceHandler
	}
)

// NewController instantiates a new Etcd controller
func NewController(client *Client) *Controller {
	return &Controller{
		client:           client,
		instanceHandlers: make([]InstanceHandler, 0),
		serviceHandlers:  make([]ServiceHandler, 0),
	}
}

// Services list declarations of all services in the system
func (c *Controller) Services() ([]*model.Service, error) {
	log.Info("Services")
	var svcs []*model.Service
	for _, svc := range Services {
		svcs = append(svcs, convertService(svc))
	}
	return svcs, nil
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
	fmt.Printf("InstancesByPort hostname=%s reqSvcPort=%d labelsList=%s", hostname, reqSvcPort, labelsList)
	name, namespace := parseHostname(hostname)

	var service *Service
	for _, svc := range Services {
		if name == svc.Name && namespace == svc.Namespace {
			service = svc
			break
		}
	}

	if service == nil {
		return nil, fmt.Errorf("no found service . name=%s namespace=%s", name, namespace)
	}

	svc := convertService(service)

	svcPortEntry, exists := svc.Ports.GetByPort(reqSvcPort)
	if !exists && reqSvcPort != 0 {
		return nil, fmt.Errorf("no found svcPortEntry . reqSvcPort=%d", reqSvcPort)
	}

	var out []*model.ServiceInstance
	for _, ep := range Endpoints {
		if ep.Name == name && ep.Namespace == namespace {
			node := getNode(ep.NodeName)
			if node == nil {
				return nil, fmt.Errorf("no found node. nodeName=%s", ep.NodeName)
			}
			if !labelsList.HasSubsetOf(node.Labels) {
				continue
			}
			// identify the port by name. K8S EndpointPort uses the service port name
			for _, port := range ep.Ports {
				if svcPortEntry.Name == port.Name {
					out = append(out, &model.ServiceInstance{
						Endpoint: model.NetworkEndpoint{
							Address:     node.Ip,
							Port:        int(port.Port),
							ServicePort: svcPortEntry,
						},
						Service:          svc,
						Labels:           node.Labels,
						AvailabilityZone: node.Az,
					})
				}
			}
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
	wch := c.client.Watch(ctx, c.client.servicePrefix(), etcd.WithPrefix())
	och := c.client.Watch(ctx, c.client.onlineRoot(), etcd.WithPrefix())

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

				if _, ok := c.client.splitServiceKey(k); ok {
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

				if _, _, ok := c.client.splitInstanceKey(k); ok {
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

				if _, _, _, ok := c.client.splitOnlineKey(k); ok {
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
