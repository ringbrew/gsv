package discovery

type Discovery interface {
	ServiceNode(name string, nodeType Type) []Node
}

type Register interface {
	Register(node *Node) error
	KeepAlive(node *Node) error
	Deregister(node *Node) error
}
