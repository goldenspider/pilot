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
	"istio.io/istio/pilot/pkg/model"
)

type (
	Client struct {
		*clientv3.Client
		prefix                string
		serviceDiscoverPrefix string
	}
)

func NewClient(client *clientv3.Client, prefix string, sdprefix string) *Client {
	return &Client{client, prefix, sdprefix}
}

func (c *Client) servicePrefix() string {
	return fmt.Sprintf("%s/services/", c.prefix)
}

func (c *Client) serviceKey(service string) string {
	return fmt.Sprintf("%s/services/%s", c.prefix, service)
}

func (c *Client) splitServiceKey(key string) (string, bool) {
	path := strings.TrimPrefix(key, c.prefix)
	parts := strings.Split(path, "/")
	if 3 == len(parts) && "services" == parts[1] {
		return parts[2], true
	} else {
		return "", false
	}
}

func (c *Client) instancePrefix(service string) string {
	return fmt.Sprintf("%s/services/%s/instances/", c.prefix, service)
}

func (c *Client) instanceKey(service string, endpoint model.NetworkEndpoint) string {
	return fmt.Sprintf("%s/services/%s/instances/%s:%d",
		c.prefix, service, endpoint.Address, endpoint.Port)
}

func (c *Client) splitInstanceKey(key string) (string, string, bool) {
	path := strings.TrimPrefix(key, c.prefix)
	parts := strings.Split(path, "/")
	if 5 == len(parts) && "services" == parts[1] && "instances" == parts[3] {
		return parts[2], parts[4], true
	} else {
		return "", "", false
	}
}

func (c *Client) onlineRoot() string {
	return fmt.Sprintf("%s/", c.serviceDiscoverPrefix)
}

func (c *Client) onlinePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", c.serviceDiscoverPrefix, service)
}

func (c *Client) splitOnlineKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.serviceDiscoverPrefix)
	parts := strings.Split(path, "/")
	if 4 == len(parts) {
		return parts[1], parts[2], parts[3], true
	} else {
		return "", "", "", false
	}
}

func (c *Client) GetServiceNames() ([]string, error) {
	key := c.servicePrefix()
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	rsp, err := c.Get(ctx, key, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		glog.Errorf("failed to get service keys. key = %s, error = %v", key, err)
		return nil, err
	}

	services := make([]string, 0, len(rsp.Kvs))
	for _, kv := range rsp.Kvs {
		name := strings.TrimPrefix(string(kv.Key), key)
		if -1 == strings.Index(name, "/") {
			services = append(services, name)
		}
	}
	glog.Infof("get service names : %v", services)
	return services, nil
}

func (c *Client) DeleteService(service string) error {
	glog.Infof("delete service, service = %s", service)

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	key := c.serviceKey(service)
	if _, err := c.Delete(ctx, key); err != nil {
		glog.Errorf("failed to delete service. key = %s, error = %v", key, err)
		return err
	}

	glog.Infof("deleted service, service = %s", service)
	return nil
}

func (c *Client) DeleteInstance(service string, endpoint model.NetworkEndpoint) error {
	glog.Infof("delete instance, service = %s, endpoint = %#v", service, endpoint)

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	key := c.instanceKey(service, endpoint)
	if _, err := c.Delete(ctx, key); err != nil {
		glog.Errorf("failed to delete service. key = %s, error = %v", key, err)
		return err
	}
	glog.Infof("delete instance, key = %s", key)
	return nil
}

func (c *Client) configPrefix() string {
	return fmt.Sprintf("%s/configs/", c.prefix)
}

func (c *Client) configTypePrefix(typ string) string {
	return fmt.Sprintf("%s/configs/%s/", c.prefix, typ)
}

func (c *Client) configNamespacePrefix(typ string, namespace string) string {
	return fmt.Sprintf("%s/configs/%s/%s/", c.prefix, typ, namespace)
}

func (c *Client) configKey(typ, name, namespace string) string {
	return fmt.Sprintf("%s/configs/%s/%s/%s", c.prefix, typ, namespace, name)
}

func (c *Client) splitConfigKey(key string) (string, string, string, bool) {
	path := strings.TrimPrefix(key, c.prefix)
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

	key := c.configKey(typ, name, namespace)
	_, err := c.Client.Delete(ctx, key)
	if err != nil {
		glog.Errorf("failed to delete config. key = %s, error = %v", key, err)
		return err
	}
	glog.Infof("delete config, key = %s", key)
	return nil
}

// parseHostname extracts service name from the service hostname
func parseHostname(hostname string) (name string, err error) {
	parts := strings.Split(hostname, ".")
	if len(parts) < 1 || parts[0] == "" {
		err = fmt.Errorf("missing service name from the service hostname %q", hostname)
		return
	}
	name = parts[0]
	return
}
