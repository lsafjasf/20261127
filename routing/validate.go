package routing

import (
	"fmt"
	"strings"
)

// IssueCode is stable enough for callers to branch on diagnostics.
type IssueCode string

const (
	IssueMissingNode     IssueCode = "missing_node"
	IssueMissingPort     IssueCode = "missing_port"
	IssueInvalidEndpoint IssueCode = "invalid_endpoint"
	IssueInvalidRoute    IssueCode = "invalid_route"
	IssueDuplicateID     IssueCode = "duplicate_id"
	IssueInvalidID       IssueCode = "invalid_id"
	IssueCycle           IssueCode = "cycle"
	IssueInvalidOp       IssueCode = "invalid_operation"
)

// Issue describes one problem, with stable IDs instead of display names.
type Issue struct {
	Code           IssueCode `json:"code"`
	Message        string    `json:"message"`
	OperationIndex int       `json:"operation,omitempty"`
	RouteID        string    `json:"route_id,omitempty"`
	NodeID         string    `json:"node_id,omitempty"`
	PortID         string    `json:"port_id,omitempty"`
	Cycle          []string  `json:"cycle,omitempty"`
	RouteIDs       []string  `json:"cycle_routes,omitempty"`
}

// DiagnosticsError aggregates every problem found in one validation pass.
type DiagnosticsError struct {
	Issues []Issue `json:"issues"`
}

func (e *DiagnosticsError) Error() string {
	if len(e.Issues) == 0 {
		return "routing validation failed"
	}
	if len(e.Issues) == 1 {
		return e.Issues[0].Message
	}
	return fmt.Sprintf("%s (and %d more issue(s))", e.Issues[0].Message, len(e.Issues)-1)
}

type nodeKind string

const (
	nodeTrack nodeKind = "track"
	nodeBus   nodeKind = "bus"
)

type indexedNode struct {
	kind    nodeKind
	inputs  []Port
	outputs []Port
}

type workspaceIndex struct {
	nodes  map[string]indexedNode
	issues []Issue
	nodeID map[string]bool
}

func (w Workspace) buildIndex() workspaceIndex {
	index := workspaceIndex{
		nodes:  make(map[string]indexedNode),
		nodeID: make(map[string]bool),
	}
	for _, track := range w.Tracks {
		index.addNode(track.ID, nodeTrack, nil, track.Outputs)
		index.validatePorts("track", track.ID, "output", track.Outputs)
	}
	for _, bus := range w.Buses {
		index.addNode(bus.ID, nodeBus, bus.Inputs, bus.Outputs)
		index.validatePorts("bus", bus.ID, "input", bus.Inputs)
		index.validatePorts("bus", bus.ID, "output", bus.Outputs)
	}
	return index
}

func (i *workspaceIndex) addNode(id string, kind nodeKind, inputs, outputs []Port) {
	if strings.TrimSpace(id) == "" {
		i.issues = append(i.issues, Issue{
			Code:    IssueInvalidID,
			Message: fmt.Sprintf("%s has an empty node ID", kind),
			NodeID:  id,
		})
		return
	}
	if i.nodeID[id] {
		i.issues = append(i.issues, Issue{
			Code:    IssueDuplicateID,
			Message: fmt.Sprintf("node ID %q is used more than once", id),
			NodeID:  id,
		})
		return
	}
	i.nodeID[id] = true
	i.nodes[id] = indexedNode{kind: kind, inputs: inputs, outputs: outputs}
}

func (i *workspaceIndex) validatePorts(kind, nodeID, direction string, ports []Port) {
	seen := make(map[string]bool, len(ports))
	for _, port := range ports {
		if strings.TrimSpace(port.ID) == "" {
			i.issues = append(i.issues, Issue{
				Code:    IssueInvalidID,
				Message: fmt.Sprintf("%s %q has an empty %s port ID", kind, nodeID, direction),
				NodeID:  nodeID,
			})
			continue
		}
		if seen[port.ID] {
			i.issues = append(i.issues, Issue{
				Code:    IssueDuplicateID,
				Message: fmt.Sprintf("%s %q repeats %s port ID %q", kind, nodeID, direction, port.ID),
				NodeID:  nodeID,
				PortID:  port.ID,
			})
			continue
		}
		seen[port.ID] = true
	}
}

func (w Workspace) validateRoute(route Route, index workspaceIndex) []Issue {
	var issues []Issue
	if strings.TrimSpace(route.ID) == "" {
		issues = append(issues, Issue{
			Code:    IssueInvalidID,
			Message: "route has an empty route ID",
			RouteID: route.ID,
		})
	}

	source, sourceIssues := validateSource(route, index)
	issues = append(issues, sourceIssues...)
	_, destinationIssues := validateDestination(route, index)
	issues = append(issues, destinationIssues...)

	if route.Kind == TrackPatch && source.kind != "" && source.kind != nodeTrack {
		issues = append(issues, Issue{
			Code:    IssueInvalidEndpoint,
			Message: fmt.Sprintf("route %q: track_patch source %q must be a track", route.ID, route.From.NodeID),
			RouteID: route.ID,
			NodeID:  route.From.NodeID,
			PortID:  route.From.PortID,
		})
	}
	if route.Kind != TrackPatch && route.Kind != Send {
		issues = append(issues, Issue{
			Code:    IssueInvalidRoute,
			Message: fmt.Sprintf("route %q: unknown route kind %q", route.ID, route.Kind),
			RouteID: route.ID,
		})
	}
	return issues
}

func validateSource(route Route, index workspaceIndex) (indexedNode, []Issue) {
	node, ok := index.nodes[route.From.NodeID]
	if !ok {
		return indexedNode{}, []Issue{{
			Code:    IssueMissingNode,
			Message: fmt.Sprintf("route %q: source node %q does not exist", route.ID, route.From.NodeID),
			RouteID: route.ID,
			NodeID:  route.From.NodeID,
			PortID:  route.From.PortID,
		}}
	}
	if strings.TrimSpace(route.From.PortID) == "" || !portExists(node.outputs, route.From.PortID) {
		return node, []Issue{{
			Code:    IssueMissingPort,
			Message: fmt.Sprintf("route %q: source port %s does not exist", route.ID, route.From),
			RouteID: route.ID,
			NodeID:  route.From.NodeID,
			PortID:  route.From.PortID,
		}}
	}
	return node, nil
}

func validateDestination(route Route, index workspaceIndex) (indexedNode, []Issue) {
	node, ok := index.nodes[route.To.NodeID]
	if !ok {
		return indexedNode{}, []Issue{{
			Code:    IssueMissingNode,
			Message: fmt.Sprintf("route %q: destination node %q does not exist", route.ID, route.To.NodeID),
			RouteID: route.ID,
			NodeID:  route.To.NodeID,
			PortID:  route.To.PortID,
		}}
	}
	if node.kind != nodeBus {
		return node, []Issue{{
			Code:    IssueInvalidEndpoint,
			Message: fmt.Sprintf("route %q: destination %q must be an output bus", route.ID, route.To.NodeID),
			RouteID: route.ID,
			NodeID:  route.To.NodeID,
			PortID:  route.To.PortID,
		}}
	}
	if strings.TrimSpace(route.To.PortID) == "" || !portExists(node.inputs, route.To.PortID) {
		return node, []Issue{{
			Code:    IssueMissingPort,
			Message: fmt.Sprintf("route %q: destination port %s does not exist", route.ID, route.To),
			RouteID: route.ID,
			NodeID:  route.To.NodeID,
			PortID:  route.To.PortID,
		}}
	}
	return node, nil
}

// Diagnostics validates the full graph without mutating it.
func (w Workspace) Diagnostics() []Issue {
	index := w.buildIndex()
	issues := append([]Issue(nil), index.issues...)
	validRoutes := make([]Route, 0, len(w.Routes))
	seenRoutes := make(map[string]bool, len(w.Routes))

	for _, route := range w.Routes {
		if seenRoutes[route.ID] {
			issues = append(issues, Issue{
				Code:    IssueDuplicateID,
				Message: fmt.Sprintf("route ID %q is used more than once", route.ID),
				RouteID: route.ID,
			})
			continue
		}
		seenRoutes[route.ID] = true
		routeIssues := w.validateRoute(route, index)
		issues = append(issues, routeIssues...)
		if len(routeIssues) == 0 {
			validRoutes = append(validRoutes, route)
		}
	}
	issues = append(issues, detectCycles(validRoutes)...)
	return issues
}

// Validate returns nil only when all nodes, ports, and routes are resolvable
// and the route graph is acyclic.
func (w Workspace) Validate() error {
	issues := w.Diagnostics()
	if len(issues) == 0 {
		return nil
	}
	return &DiagnosticsError{Issues: issues}
}

type cycleEdge struct {
	to      string
	routeID string
}

func detectCycles(routes []Route) []Issue {
	adjacency := make(map[string][]cycleEdge)
	var order []string
	seenNode := make(map[string]bool)
	for _, route := range routes {
		if !seenNode[route.From.NodeID] {
			seenNode[route.From.NodeID] = true
			order = append(order, route.From.NodeID)
		}
		adjacency[route.From.NodeID] = append(adjacency[route.From.NodeID], cycleEdge{
			to:      route.To.NodeID,
			routeID: route.ID,
		})
	}

	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[string]int)
	var nodeStack []string
	var routeStack []string
	var issues []Issue
	seenCycle := make(map[string]bool)

	var visit func(string)
	visit = func(node string) {
		state[node] = visiting
		nodeStack = append(nodeStack, node)
		for _, edge := range adjacency[node] {
			switch state[edge.to] {
			case unvisited:
				routeStack = append(routeStack, edge.routeID)
				visit(edge.to)
				routeStack = routeStack[:len(routeStack)-1]
			case visiting:
				start := 0
				for start < len(nodeStack) && nodeStack[start] != edge.to {
					start++
				}
				if start == len(nodeStack) {
					continue
				}
				cycleNodes := append([]string(nil), nodeStack[start:]...)
				cycleNodes = append(cycleNodes, edge.to)
				cycleRoutes := append([]string(nil), routeStack[start:]...)
				cycleRoutes = append(cycleRoutes, edge.routeID)
				key := canonicalCycle(cycleNodes)
				if seenCycle[key] {
					continue
				}
				seenCycle[key] = true
				issues = append(issues, Issue{
					Code:     IssueCycle,
					Message:  "routing cycle: " + strings.Join(cycleNodes, " -> "),
					Cycle:    cycleNodes,
					RouteIDs: cycleRoutes,
				})
			}
		}
		nodeStack = nodeStack[:len(nodeStack)-1]
		state[node] = done
	}

	for _, node := range order {
		if state[node] == unvisited {
			visit(node)
		}
	}
	return issues
}

func canonicalCycle(nodes []string) string {
	if len(nodes) < 2 {
		return strings.Join(nodes, "\x00")
	}
	body := nodes[:len(nodes)-1]
	best := 0
	for i := 1; i < len(body); i++ {
		if strings.Join(body[i:], "\x00")+"\x00"+strings.Join(body[:i], "\x00") <
			strings.Join(body[best:], "\x00")+"\x00"+strings.Join(body[:best], "\x00") {
			best = i
		}
	}
	rotated := append(append([]string(nil), body[best:]...), body[:best]...)
	return strings.Join(rotated, "\x00")
}
