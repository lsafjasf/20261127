package ebml

import (
	"bytes"
	"fmt"
)

// Node is an editable EBML element. Parsed containers have Children non-nil;
// leaf and unknown elements retain exact Payload and are re-encoded unchanged.
type Node struct {
	ID       ID
	Payload  []byte
	Children []*Node
}

func ParseChildren(data []byte, containers map[ID]bool) ([]*Node, error) {
	var nodes []*Node
	for len(data) > 0 {
		node, rest, err := ParseNode(data, containers)
		if err != nil {
			return nodes, err
		}
		nodes = append(nodes, node)
		data = rest
	}
	return nodes, nil
}

func ParseNode(data []byte, containers map[ID]bool) (*Node, []byte, error) {
	header, err := ReadHeader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	total := header.HeaderLength + int(header.Size)
	if header.UnknownSize {
		return nil, nil, fmt.Errorf("ebml: unknown-size node in buffered parse is not supported for id %x", header.ID)
	}
	if total > len(data) {
		return nil, nil, fmt.Errorf("%w: id %x needs %d bytes, have %d", ErrTruncated, header.ID, total, len(data))
	}
	payload := data[header.HeaderLength:total]
	node := &Node{ID: header.ID, Payload: payload}
	if containers[header.ID] {
		children, err := ParseChildren(payload, containers)
		if err != nil {
			return nil, nil, fmt.Errorf("element %x: %w", header.ID, err)
		}
		node.Children = children
		node.Payload = nil
	}
	return node, data[total:], nil
}

func (n *Node) Bytes() []byte {
	var payload []byte
	if n.Children != nil {
		var buf bytes.Buffer
		for _, child := range n.Children {
			buf.Write(child.Bytes())
		}
		payload = buf.Bytes()
	} else {
		payload = n.Payload
	}
	return ElementBytes(n.ID, payload)
}

func (n *Node) First(id ID) *Node {
	for _, child := range n.Children {
		if child.ID == id {
			return child
		}
	}
	return nil
}

func (n *Node) All(id ID) []*Node {
	var out []*Node
	for _, child := range n.Children {
		if child.ID == id {
			out = append(out, child)
		}
	}
	return out
}

func (n *Node) UintValue(id ID) (uint64, bool) {
	child := n.First(id)
	if child == nil || child.Children != nil {
		return 0, false
	}
	return ReadUint(child.Payload), true
}

func (n *Node) StringValue(id ID) (string, bool) {
	child := n.First(id)
	if child == nil || child.Children != nil {
		return "", false
	}
	return string(child.Payload), true
}

// SetUint replaces the first child or appends a canonical uint element.
func (n *Node) SetUint(id ID, value uint64) *Node {
	payload := EncodeUint(value)
	if child := n.First(id); child != nil {
		child.Payload, child.Children = payload, nil
		return child
	}
	child := &Node{ID: id, Payload: payload}
	n.Children = append(n.Children, child)
	return child
}

func (n *Node) SetString(id ID, value string) *Node {
	payload := EncodeString(value)
	if child := n.First(id); child != nil {
		child.Payload, child.Children = payload, nil
		return child
	}
	child := &Node{ID: id, Payload: payload}
	n.Children = append(n.Children, child)
	return child
}

func (n *Node) RemoveAll(id ID) int {
	kept := n.Children[:0]
	removed := 0
	for _, child := range n.Children {
		if child.ID == id {
			removed++
			continue
		}
		kept = append(kept, child)
	}
	n.Children = kept
	return removed
}

func NewUintNode(id ID, value uint64) *Node {
	return &Node{ID: id, Payload: EncodeUint(value)}
}

func NewStringNode(id ID, value string) *Node {
	return &Node{ID: id, Payload: []byte(value)}
}

func NewContainer(id ID, children ...*Node) *Node {
	return &Node{ID: id, Children: children}
}
