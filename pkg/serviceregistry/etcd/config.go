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
	"fmt"

	"context"
	etcd "github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"istio.io/istio/pilot/pkg/model"
	"sync"

	"sync/atomic"
	"time"
)

// Handler specifies a function to apply on a Config for a given event type
type Handler = func(model.Config, model.Event)

type configStoreCache struct {
	client   *Client
	handlers map[string][]Handler
	mu       sync.RWMutex
	cfgCache map[string]*configInfo
}

type configInfo struct {
	hit  uint64
	miss uint64
	cfgs []model.Config
}

// ConfigStoreCache instantiates the Eureka service account interface
func NewConfigStoreCache(client *Client) model.ConfigStoreCache {
	return &configStoreCache{
		client:   client,
		handlers: map[string][]Handler{},
		cfgCache: make(map[string]*configInfo),
	}
}

// ConfigDescriptor exposes the configuration type schema known by the config store.
// The type schema defines the bidrectional mapping between configuration
// types and the protobuf encoding schema.
func (c *configStoreCache) ConfigDescriptor() model.ConfigDescriptor {
	return model.IstioConfigTypes
}

func (c *configStoreCache) stats() map[string]*configInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make(map[string]*configInfo, len(c.cfgCache))
	for k, v := range c.cfgCache {
		stats[k] = &configInfo{
			hit:  atomic.LoadUint64(&v.hit),
			miss: atomic.LoadUint64(&v.miss),
		}
	}
	return stats
}

func (c *configStoreCache) printStats() {
	cfgCache := c.stats()
	for k, v := range cfgCache {
		glog.Infof("configCache key:%s hit:%d miss:%d", k, v.hit, v.miss)
	}
}

func (c *configStoreCache) clearCfgCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range c.cfgCache {
		v.cfgs = nil
	}
}

func (c *configStoreCache) cachedCfgCache(key string) ([]model.Config, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cfgCache[key]
	if !ok || entry.cfgs == nil {
		return nil, false
	}

	// Hit
	atomic.AddUint64(&entry.hit, 1)
	return entry.cfgs, true
}

func (c *configStoreCache) updateCfgCache(key string, data []model.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.cfgCache[key]

	if !ok {
		entry = &configInfo{}
		c.cfgCache[key] = entry
	} else if entry.cfgs != nil {
		glog.Warningf("Overriding config cached data for entry %v", key)
	}
	entry.cfgs = data
	atomic.AddUint64(&entry.miss, 1)
}

// Get retrieves a configuration element by a type and a key
func (c *configStoreCache) Get(typ, name, namespace string) (config *model.Config, exists bool) {
	var err error
	key := c.client.configKey(typ, name, namespace)
	list, cached := c.cachedCfgCache(key)
	if !cached {
		if config, err = c.client.GetConfig(typ, name, namespace); err != nil {
			exists = false
			glog.Warning(err)
			return
		}

		//return list, nil
		c.updateCfgCache(key, []model.Config{*config})

	} else {
		config = &list[0]
	}
	exists = true
	return
}

// List returns objects by type and namespace.
// Use "" for the namespace to list across namespaces.
func (c *configStoreCache) List(typ, namespace string) ([]model.Config, error) {
	glog.Infof("List typ=%s namespace=%s", typ, namespace)
	if typ == model.VirtualService.Type {
		varr, err := ParseInputs(vsdata)
		if err != nil || len(varr) == 0 {
			fmt.Printf("ParseInputs(correct input) => got %v, %v", varr, err)
		}

		varr_alpha, err := ParseInputs(vsdata_alpha)
		if err != nil || len(varr_alpha) == 0 {
			fmt.Printf("ParseInputs(correct input) => got %v, %v", varr_alpha, err)
		}

		varr = append(varr, varr_alpha...)
		fmt.Printf("virtual-service Ok: varr:%+v\n", varr)
		return varr, nil
	}

	if typ == model.DestinationRule.Type {
		varr, err := ParseInputs(drdata)
		if err != nil || len(varr) == 0 {
			fmt.Printf("ParseInputs(correct input) => got %v, %v", varr, err)
		}
		varr_alpha, err := ParseInputs(drdata_alpha)
		if err != nil || len(varr_alpha) == 0 {
			fmt.Printf("ParseInputs(correct input) => got %v, %v", varr_alpha, err)
		}
		varr = append(varr, varr_alpha...)
		fmt.Printf("destination-rule Ok: varr:%+v\n", varr)
		return varr, nil
	}

	return nil, nil
}

// Create adds a new configuration object to the store. If an object with the
// same name and namespace for the type already exists, the operation fails
// with no side effects.
func (c *configStoreCache) Create(config model.Config) (revision string, err error) {
	if err = c.client.CreateConfig(&config); err != nil {
		return
	}
	revision = config.ResourceVersion
	return
}

// Update modifies an existing configuration object in the store.  Update
// requires that the object has been created.  Resource version prevents
// overriding a value that has been changed between prior _Get_ and _Put_
// operation to achieve optimistic concurrency. This method returns a new
// revision if the operation succeeds.
func (c *configStoreCache) Update(config model.Config) (newRevision string, err error) {
	if err = c.client.UpdateConfig(&config); err != nil {
		return
	}
	newRevision = config.ResourceVersion
	return
}

// Delete removes an object from the store by key
func (c *configStoreCache) Delete(typ, name, namespace string) error {
	_, exists := c.ConfigDescriptor().GetByType(typ)
	if !exists {
		return fmt.Errorf("missing type %q", typ)
	}
	return c.client.DeleteConfig(typ, name, namespace)
}

// RegisterEventHandler adds a handler to receive config update events for a
// configuration type
func (c *configStoreCache) RegisterEventHandler(typ string, handler Handler) {
	if _, exists := c.handlers[typ]; exists {
		c.handlers[typ] = append(c.handlers[typ], handler)
	} else {
		c.handlers[typ] = []Handler{handler}
	}
}

// Run until a signal is received
func (c *configStoreCache) Run(stop <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	key := c.client.configPrefix()
	wch := c.client.Watch(ctx, key, etcd.WithPrefix())
	ticker := time.NewTicker(2 * time.Minute)

	for {
		select {
		case <-stop:
			cancel()
			ticker.Stop()
			return
		case <-ticker.C:
			go c.printStats()
			go c.clearCfgCache()
		case wresp := <-wch:
			go c.clearCfgCache()
			for _, event := range wresp.Events {
				k := string(event.Kv.Key)

				if typ, _, _, ok := c.client.splitConfigKey(k); ok {
					if handles, ok := c.handlers[typ]; ok {
						switch event.Type {
						case etcd.EventTypePut:
							cfg := model.Config{}
							for _, h := range handles {
								go h(cfg, model.EventAdd)
							}

						case etcd.EventTypeDelete:
							cfg := model.Config{}
							for _, h := range handles {
								go h(cfg, model.EventDelete)
							}
						}
					}
				}
			}
		}
	}
}

// HasSynced returns true after initial cache synchronization is complete
func (c *configStoreCache) HasSynced() bool {
	return true
}
