package discovery

type NodeDiscover interface {
	Node(name string, nodeType Type) []Node
	Watch(name string, nodeType Type) chan NodeEvent
}

type NodeRegister interface {
	Register(node *Node) error
	KeepAlive(node *Node) error
	Deregister(node *Node) error
}

type NodeEventType int

const (
	NodeEventAdd NodeEventType = iota + 1
	NodeEventRemove
	NodeEventSync
)

type NodeEvent struct {
	Event NodeEventType
	Node  []Node
}
