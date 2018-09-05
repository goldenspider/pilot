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
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"go.uber.org/zap"
	"istio.io/istio/pilot/pkg/model"
	"net"
	"strconv"
)

type (
	Client struct {
		*zap.SugaredLogger
		*clientv3.Client
		Prefix                string
		ServiceDiscoverPrefix string
	}
)

func NewClient(l *zap.SugaredLogger, client *clientv3.Client, prefix string, sdprefix string) *Client {
	return &Client{SugaredLogger: l.Named("Client"), Client: client, Prefix: prefix, ServiceDiscoverPrefix: sdprefix}
}

func (c *Client) ServicePrefix() string {
	return fmt.Sprintf("%s/services/", c.Prefix)
}

func (c *Client) ServiceKey(service string) string {
	return fmt.Sprintf("%s/services/%s", c.Prefix, service)
}

func (c *Client) SplitServiceKey(key string) (string, bool) {
	path := strings.TrimPrefix(key, c.Prefix)
	parts := strings.Split(path, "/")
	if 3 == len(parts) && "services" == parts[1] {
		return parts[2], true
	} else {
		return "", false
	}
}

func (c *Client) InstancePrefix(service string) string {
	return fmt.Sprintf("%s/services/%s/instances/", c.Prefix, service)
}

func (c *Client) InstanceKey(service string, endpoint model.NetworkEndpoint) string {
	return fmt.Sprintf("%s/services/%s/instances/%s:%d",
		c.Prefix, service, endpoint.Address, endpoint.Port)
}

func (c *Client) SplitInstanceKey(key string) (string, string, bool) {
	path := strings.TrimPrefix(key, c.Prefix)
	parts := strings.Split(path, "/")
	if 5 == len(parts) && "services" == parts[1] && "instances" == parts[3] {
		return parts[2], parts[4], true
	} else {
		return "", "", false
	}
}

func (c *Client) OnlineRoot() string {
	return fmt.Sprintf("%s/", c.ServiceDiscoverPrefix)
}

func (c *Client) OnlinePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", c.ServiceDiscoverPrefix, service)
}

func (c *Client) SplitOnlineKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.ServiceDiscoverPrefix)
	parts := strings.Split(path, "/")
	if 4 == len(parts) {
		return parts[1], parts[2], parts[3], true
	} else {
		return "", "", "", false
	}
}

func (c *Client) GetServiceNames() ([]string, error) {
	key := c.ServicePrefix()
	glog.Infof("key=%s", key)
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	rsp, err := c.Get(ctx, key, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		glog.Errorf("failed to get service keys. key = %s, error = %v", key, err)
		return nil, err
	}

	glog.Infof("rsp=%+v", *rsp)
	services := make([]string, 0, len(rsp.Kvs))
	for _, kv := range rsp.Kvs {
		glog.Infof("kv =%+v", *kv)
		name := strings.TrimPrefix(string(kv.Key), key)
		if -1 == strings.Index(name, "/") {
			services = append(services, name)
		}
	}
	glog.Infof("get service names : %v", services)
	return services, nil
}

// GetService retrieves a service by host name if it exists
func (c *Client) GetService(hostname string) (*model.Service, error) {
	glog.Infof("get services, hostname = %s", hostname)
	// Get actual service by name
	//name, namespace := parseHostname(hostname)

	key := c.ServiceKey(hostname)
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	rsp, err := c.Get(ctx, key)
	if err != nil || 0 == len(rsp.Kvs) {
		glog.Errorf("failed to get services. key = %s, error = %v", key, err)
		return nil, err
	}

	kv := rsp.Kvs[0]
	obj, err := UnmarshalServiceFromYAML(kv.Value)
	if err != nil {
		glog.Errorf("failed to parse service. key = %s, value = %s, error = %v",
			string(kv.Key), string(kv.Value), err)
		return nil, err
	}
	glog.Infof("get services, hostname = %s, key = %s, value = %s, service = %#v ",
		hostname, key, string(kv.Value), obj)
	return obj, nil
}

func (c *Client) DeleteService(service string) error {
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

func (c *Client) onlineRoot() string {
	return fmt.Sprintf("%s/", c.ServiceDiscoverPrefix)
}

func (c *Client) onlinePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", c.ServiceDiscoverPrefix, service)
}

func (c *Client) splitOnlineKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.ServiceDiscoverPrefix)
	parts := strings.Split(path, "/")
	if 4 == len(parts) {
		return parts[1], parts[2], parts[3], true
	} else {
		return "", "", "", false
	}
}

func (c *Client) instancePrefix(service string) string {
	return fmt.Sprintf("%s/services/%s/instances/", c.Prefix, service)
}

func (c *Client) instanceKey(service string, endpoint model.NetworkEndpoint) string {
	return fmt.Sprintf("%s/services/%s/instances/%s:%d",
		c.Prefix, service, endpoint.Address, endpoint.Port)
}

func (c *Client) splitInstanceKey(key string) (string, string, bool) {
	path := strings.TrimPrefix(key, c.Prefix)
	parts := strings.Split(path, "/")
	if 5 == len(parts) && "services" == parts[1] && "instances" == parts[3] {
		return parts[2], parts[4], true
	} else {
		return "", "", false
	}
}

func (c *Client) GetInstances(hostname string) ([]*model.ServiceInstance, error) {
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
	c.Infof("get instances, online = %v", online)

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
			c.Errorf("failed to parse instance. key = %s, value = %s, error = %v",
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

		c.Infof("get instance, key = %s, value = %s, instance = %#v", instanceKey, string(kv.Value), obj)
		instances = append(instances, obj)
	}

	return instances, nil
}

func (c *Client) ConfigPrefix() string {
	return fmt.Sprintf("%s/configs/", c.Prefix)
}

func (c *Client) ConfigTypePrefix(typ string) string {
	return fmt.Sprintf("%s/configs/%s/", c.Prefix, typ)
}

func (c *Client) ConfigNamespacePrefix(typ string, namespace string) string {
	return fmt.Sprintf("%s/configs/%s/%s/", c.Prefix, typ, namespace)
}

func (c *Client) ConfigKey(typ, name, namespace string) string {
	return fmt.Sprintf("%s/configs/%s/%s/%s", c.Prefix, typ, namespace, name)
}

func (c *Client) SplitConfigKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.Prefix)
	parts := strings.Split(path, "/")
	if 5 == len(parts) && "configs" == parts[1] {
		return parts[2], parts[3], parts[4], true
	} else {
		return "", "", "", false
	}
}

func (c *Client) GetConfig(typ, name, namespace string) (*model.Config, error) {

	return nil, nil
}

func (c *Client) ListConfigs(typ string, namespace string) ([]model.Config, error) {

	return nil, nil
}

func (c *Client) CreateConfig(config *model.Config) error {

	return nil
}

// Update modifies an existing configuration object in the store.  Update
// requires that the object has been created.  Resource version prevents
// overriding a value that has been changed between prior _Get_ and _Put_
// operation to achieve optimistic concurrency. This method returns a new
// revision if the operation succeeds.
func (c *Client) UpdateConfig(config *model.Config) error {
	return c.CreateConfig(config)
}

// Delete removes an object from the store by key
func (c *Client) DeleteConfig(typ, name, namespace string) error {
	glog.Infof("delete config, type = %s, name = %s, namespace = %s", typ, name, namespace)

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	key := c.ConfigKey(typ, name, namespace)
	_, err := c.Client.Delete(ctx, key)
	if err != nil {
		glog.Errorf("failed to delete config. key = %s, error = %v", key, err)
		return err
	}
	glog.Infof("delete config, key = %s", key)
	return nil
}
