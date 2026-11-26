package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type xmlNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Nodes   []xmlNode  `xml:",any"`
}

func readXML(path string) (*xmlNode, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	var root xmlNode
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse XML %s: %w", path, err)
	}
	return &root, nil
}

func (n *xmlNode) localName() string {
	if n == nil {
		return ""
	}
	return n.XMLName.Local
}

func (n *xmlNode) child(name string) *xmlNode {
	if n == nil {
		return nil
	}
	for i := range n.Nodes {
		if n.Nodes[i].localName() == name {
			return &n.Nodes[i]
		}
	}
	return nil
}

func (n *xmlNode) children(name string) []xmlNode {
	if n == nil {
		return nil
	}
	var result []xmlNode
	for _, node := range n.Nodes {
		if node.localName() == name {
			result = append(result, node)
		}
	}
	return result
}

func (n *xmlNode) childText(name string) string {
	child := n.child(name)
	if child == nil {
		return ""
	}
	return strings.TrimSpace(child.Text)
}

func (n *xmlNode) childInt(name string) (int64, bool, error) {
	raw := n.childText(name)
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, true, fmt.Errorf("invalid %s %q: %w", name, raw, err)
	}
	return value, true, nil
}

func normalizeID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "urn:uuid:")
	return strings.ToLower(value)
}

func looksLikeCPL(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(name, "cpl_") && strings.HasSuffix(name, ".xml")
}

func findFiles(root string, match func(string) bool) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if match(path) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func firstFile(root string, match func(string) bool) (string, error) {
	paths, err := findFiles(root, match)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", nil
	}
	return paths[0], nil
}
