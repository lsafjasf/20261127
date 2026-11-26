package imf

import (
	"encoding/xml"
	"io"
	"strings"
)

// Node is a namespace-agnostic XML element tree. IMF documents use several
// schema generations and prefixes (cc:, pkl:, assetmap: ...); matching on the
// local name keeps the loader tolerant of all of them.
type Node struct {
	Local    string
	Space    string
	Attrs    []xml.Attr
	Children []*Node
	Text     string
}

// ParseNode decodes one XML document into a Node tree.
func ParseNode(r io.Reader) (*Node, error) {
	dec := xml.NewDecoder(r)
	var root *Node
	var stack []*Node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &Node{Local: t.Name.Local, Space: t.Name.Space, Attrs: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) == 0 {
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				s := strings.TrimSpace(string(t))
				if s != "" {
					cur := stack[len(stack)-1]
					if cur.Text == "" {
						cur.Text = s
					} else {
						cur.Text += " " + s
					}
				}
			}
		}
	}
	return root, nil
}

// Attr returns the first attribute whose local name matches.
func (n *Node) Attr(name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

// First returns the first descendant/child element with the given local name.
func (n *Node) First(name string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if strings.EqualFold(c.Local, name) {
			return c
		}
	}
	for _, c := range n.Children {
		if f := c.First(name); f != nil {
			return f
		}
	}
	return nil
}

// All returns all descendant elements with the given local name.
func (n *Node) All(name string) []*Node {
	var out []*Node
	if n == nil {
		return out
	}
	var walk func(*Node)
	walk = func(cur *Node) {
		for _, c := range cur.Children {
			if strings.EqualFold(c.Local, name) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// AllAny returns descendants matching any of the provided local names.
func (n *Node) AllAny(names ...string) []*Node {
	want := map[string]bool{}
	for _, x := range names {
		want[strings.ToLower(x)] = true
	}
	var out []*Node
	var walk func(*Node)
	walk = func(cur *Node) {
		for _, c := range cur.Children {
			if want[strings.ToLower(c.Local)] {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// ChildText returns the trimmed text of the first matching child.
func (n *Node) ChildText(name string) string {
	f := n.First(name)
	if f == nil {
		return ""
	}
	return strings.TrimSpace(f.Text)
}

// DirectChildren returns immediate children (not descendants).
func (n *Node) DirectChildren() []*Node {
	if n == nil {
		return nil
	}
	return n.Children
}
