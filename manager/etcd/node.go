package etcd

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	pb "pilot/pkg/proto/etcd"
)

type NodeManager struct {
	*zap.SugaredLogger
	ds *DataSource
}

func NewNodeManager(l *zap.SugaredLogger, ds *DataSource) *NodeManager {
	return &NodeManager{SugaredLogger: l.Named("NodeManager"), ds: ds}
}

func (m *NodeManager) initRouter(r *gin.RouterGroup) error {
	n := &pb.Node{}
	if e := m.ds.initRouter(r, "/nodes", n); e != nil {
		return e
	}

	return nil
}

func (m *NodeManager) FindNodeByIP(ip string) *pb.Node {
	o := &pb.Node{}
	objs, e := m.ds.List("/nodes/", o)
	if e != nil {
		m.Errorf("failed to list nodes. err = %v", e)
		return nil
	}
	for _, obj := range objs {
		node := obj.(*pb.Node)
		if node.Ip == ip {
			return node
		}
	}
	return nil
}

func (m *NodeManager) GetNode(nodeId string) *pb.Node {
	node := &pb.Node{}
	nodeKey := fmt.Sprintf("/nodes/%s", nodeId)
	if e := m.ds.Get(nodeKey, node); e != nil {
		m.Errorf("查询Node(%s)失败. %s", nodeId, e)
		return nil
	}

	return node
}
