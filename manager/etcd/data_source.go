package etcd

import (
	"encoding/json"

	etcd "github.com/coreos/etcd/clientv3"

	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	log "github.com/golang/glog"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type (
	DataSource struct {
		*zap.SugaredLogger
		Client *etcd.Client
	}
)

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func AbortWithError(l *zap.SugaredLogger, c *gin.Context, e error) {
	err, ok := errors.Cause(e).(stackTracer)
	if ok {
		st := err.StackTrace()
		l.Infof("error: %s\n%+v", err, st[0:2])
	} else {
		l.Infof("%+v", e)
	}

	c.Abort()
	c.String(http.StatusBadGateway, "%+v", e)
}

func NewDataSource(l *zap.SugaredLogger, c *etcd.Client) *DataSource {
	return &DataSource{SugaredLogger: l.Named("DataSource"), Client: c}
}

func (m *DataSource) initRouter(r *gin.RouterGroup, path string, o interface{}) error {
	r.GET(path, func(c *gin.Context) {
		instances, e := m.List(fmt.Sprintf("%s/", path), o)
		if e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, instances)
	})

	p := fmt.Sprintf("%s/:id", path)
	r.GET(p, func(c *gin.Context) {
		id := c.Params.ByName("id")
		instancePath := fmt.Sprintf("%s/%s", path, id)

		if e := m.Get(instancePath, o); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, o)
	})

	r.PUT(p, func(c *gin.Context) {
		id := c.Params.ByName("id")
		instancePath := fmt.Sprintf("%s/%s", path, id)

		c.BindJSON(o)

		if e := m.Put(instancePath, o); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	r.PATCH(p, func(c *gin.Context) {
		id := c.Params.ByName("id")
		instancePath := fmt.Sprintf("%s/%s", path, id)

		c.BindJSON(o)

		if e := m.Patch(instancePath, o); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	r.DELETE(p, func(c *gin.Context) {
		id := c.Params.ByName("id")
		instancePath := fmt.Sprintf("%s/%s", path, id)

		if e := m.Delete(instancePath); e != nil {
			AbortWithError(m.SugaredLogger, c, e)
			return
		}
		c.JSON(200, struct{}{})
	})

	return nil
}

func (s *DataSource) List(key string, o interface{}) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	rsp, e := s.Client.Get(ctx, key, etcd.WithPrefix())
	if e != nil {
		return nil, errors.Wrapf(e, "查询ETCD失败: KEY为%s", key)
	}

	values := make([]interface{}, 0, len(rsp.Kvs))
	for i, kv := range rsp.Kvs {
		name := strings.TrimPrefix(string(kv.Key), key)
		if -1 != strings.Index(name, "/") {
			continue
		}

		v := reflect.New(reflect.TypeOf(o).Elem()).Interface()
		if e = json.Unmarshal(kv.Value, v); e != nil {
			return nil, errors.Wrapf(e, "解析JSON失败: KEY为%s#%d, 类型为%T，JSON为%s", key, i, o, rsp.Kvs[i].String())
		}
		values = append(values, v)
	}
	return values, nil
}

func (s *DataSource) Get(key string, o interface{}) error {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	rsp, e := s.Client.Get(ctx, key)
	if e != nil {
		return errors.Wrapf(e, "查询ETCD失败: KEY为%s", key)
	}

	if 1 != len(rsp.Kvs) {
		return errors.Errorf("ETCD中指定KEY(%s)不存在", key)
	}

	if e = json.Unmarshal(rsp.Kvs[0].Value, o); e != nil {
		return errors.Wrapf(e, "解析JSON失败: KEY为%s, 类型为%T，JSON为%s", key, o, rsp.Kvs[0].String())
	}

	return nil
}

func (s *DataSource) Put(key string, o interface{}) error {
	bytes, e := json.Marshal(o)
	if e != nil {
		return errors.Wrapf(e, "生成JSON失败: 对象为%#v", o)
	}
	log.Infof("write object to %s. obj = %#v, value = %s", key, o, string(bytes))

	value := string(bytes)
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	if _, e = s.Client.Put(ctx, key, value); e != nil {
		return errors.Wrapf(e, "写入ETCD失败: KEY为%s，值为%s", key, value)
	}
	return nil
}

func (s *DataSource) Patch(key string, o interface{}) error {
	v := reflect.New(reflect.TypeOf(o).Elem()).Interface()
	if e := s.Get(key, v); e != nil {
		return errors.Wrapf(e, "patch对象失败")
	}

	if e := mergo.Merge(v, o); e != nil {
		return errors.Wrapf(e, "patch对象失败: 值为%#v, Patch为%#v", v, o)
	}

	if e := s.Put(key, v); e != nil {
		return errors.Wrapf(e, "patch对象失败")
	}

	return nil
}

func (m *DataSource) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	if _, e := m.Client.Delete(ctx, key); e != nil {
		return errors.Wrapf(e, "删除ETCD中KEY(%s)失败", key)
	}
	return nil
}

func (s *DataSource) GetBytes(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	rsp, e := s.Client.Get(ctx, key)
	if e != nil {
		return nil, errors.Wrapf(e, "查询ETCD失败: KEY为%s", key)
	}

	if 1 != len(rsp.Kvs) {
		return nil, errors.Errorf("ETCD中指定KEY(%s)不存在", key)
	}

	return rsp.Kvs[0].Value, nil
}

func (s *DataSource) PutBytes(key string, value []byte) error {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	_, e := s.Client.Put(ctx, key, string(value))
	if e != nil {
		return errors.Wrapf(e, "写入ETCD失败: KEY为%s，值为%s", key, value)
	}
	return nil
}
