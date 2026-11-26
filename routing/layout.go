package routing

import (
	"fmt"
	"math"
	"strings"
)

const (
	trackX       = 48.0
	trackWidth   = 300.0
	busX         = 760.0
	busWidth     = 320.0
	graphTop     = 128.0
	nodeHeader   = 58.0
	portRow      = 30.0
	nodeBottom   = 16.0
	nodeGap      = 34.0
	minGraphWide = 1320.0
)

type portLayout struct {
	ID   string
	Text string
	X    float64
	Y    float64
}

type nodeLayout struct {
	ID      string
	Name    string
	Kind    nodeKind
	X       float64
	Y       float64
	W       float64
	H       float64
	Inputs  []portLayout
	Outputs []portLayout
}

func (n nodeLayout) right() float64 { return n.X + n.W }
func (n nodeLayout) bottom() float64 {
	return n.Y + n.H
}

type ghostLayout struct {
	ID   string
	X    float64
	Y    float64
	W    float64
	H    float64
	Kind nodeKind
}

type routeLayout struct {
	Index       int
	Route       Route
	FromX       float64
	FromY       float64
	ToX         float64
	ToY         float64
	Path        string
	LabelX      float64
	LabelY      float64
	MissingFrom bool
	MissingTo   bool
	Invalid     bool
	Cycle       bool
}

type graphLayout struct {
	Width   float64
	Height  float64
	Tracks  []nodeLayout
	Buses   []nodeLayout
	Routes  []routeLayout
	Ghosts  []ghostLayout
	NodeMap map[string]nodeLayout
}

func buildGraph(workspace Workspace, issues []Issue) graphLayout {
	graph := graphLayout{NodeMap: make(map[string]nodeLayout)}
	issuesByRoute := make(map[string][]Issue)
	cycleRoutes := make(map[string]bool)
	for _, issue := range issues {
		if issue.RouteID != "" {
			issuesByRoute[issue.RouteID] = append(issuesByRoute[issue.RouteID], issue)
		}
		for _, routeID := range issue.RouteIDs {
			cycleRoutes[routeID] = true
		}
	}

	y := graphTop
	for _, track := range workspace.Tracks {
		node := makeNode(track.ID, track.Name, nodeTrack, trackX, y, trackWidth, nil, track.Outputs)
		graph.Tracks = append(graph.Tracks, node)
		if _, exists := graph.NodeMap[track.ID]; !exists {
			graph.NodeMap[track.ID] = node
		}
		y = node.bottom() + nodeGap
	}
	trackEnd := y

	y = graphTop
	for _, bus := range workspace.Buses {
		node := makeNode(bus.ID, bus.Name, nodeBus, busX, y, busWidth, bus.Inputs, bus.Outputs)
		graph.Buses = append(graph.Buses, node)
		if _, exists := graph.NodeMap[bus.ID]; !exists {
			graph.NodeMap[bus.ID] = node
		}
		y = node.bottom() + nodeGap
	}
	busEnd := y

	index := workspace.buildIndex()
	ghostIndex := make(map[string]int)
	for _, route := range workspace.Routes {
		if _, ok := index.nodes[route.From.NodeID]; !ok {
			addGhost(&graph, ghostIndex, route.From.NodeID, nodeTrack, trackX, &trackEnd, trackWidth)
		}
		if _, ok := index.nodes[route.To.NodeID]; !ok {
			addGhost(&graph, ghostIndex, route.To.NodeID, nodeBus, busX, &busEnd, busWidth)
		}
	}

	for i, route := range workspace.Routes {
		fromX, fromY, missingFrom := graph.sourceAnchor(route)
		toX, toY, missingTo := graph.destinationAnchor(route)
		path, labelX, labelY := routePath(fromX, fromY, toX, toY, i)
		graph.Routes = append(graph.Routes, routeLayout{
			Index:       i,
			Route:       route,
			FromX:       fromX,
			FromY:       fromY,
			ToX:         toX,
			ToY:         toY,
			Path:        path,
			LabelX:      labelX,
			LabelY:      labelY,
			MissingFrom: missingFrom,
			MissingTo:   missingTo,
			Invalid:     len(issuesByRoute[route.ID]) > 0 || missingFrom || missingTo,
			Cycle:       cycleRoutes[route.ID],
		})
	}

	graph.Width = math.Max(minGraphWide, busX+busWidth+80)
	graph.Height = math.Max(trackEnd, busEnd) + 54
	return graph
}

func makeNode(id, name string, kind nodeKind, x, y, width float64, inputs, outputs []Port) nodeLayout {
	rows := max(len(inputs), len(outputs), 1)
	height := nodeHeader + float64(rows)*portRow + nodeBottom
	node := nodeLayout{ID: id, Name: name, Kind: kind, X: x, Y: y, W: width, H: height}
	for i, port := range inputs {
		node.Inputs = append(node.Inputs, portLayout{
			ID:   port.ID,
			Text: portText(port),
			X:    x,
			Y:    y + nodeHeader + portRow/2 + float64(i)*portRow,
		})
	}
	for i, port := range outputs {
		node.Outputs = append(node.Outputs, portLayout{
			ID:   port.ID,
			Text: portText(port),
			X:    x + width,
			Y:    y + nodeHeader + portRow/2 + float64(i)*portRow,
		})
	}
	return node
}

func portText(port Port) string {
	if port.Label == "" || port.Label == port.ID {
		return port.ID
	}
	return port.ID + " · " + port.Label
}

func addGhost(graph *graphLayout, seen map[string]int, id string, kind nodeKind, x float64, y *float64, width float64) {
	if id == "" {
		id = "(empty ID)"
	}
	if _, ok := seen[id]; ok {
		return
	}
	ghost := ghostLayout{ID: id, X: x, Y: *y, W: width, H: 62, Kind: kind}
	seen[id] = len(graph.Ghosts)
	graph.Ghosts = append(graph.Ghosts, ghost)
	*y += ghost.H + nodeGap
}

func (g graphLayout) sourceAnchor(route Route) (float64, float64, bool) {
	node, ok := g.NodeMap[route.From.NodeID]
	if !ok {
		for _, ghost := range g.Ghosts {
			if ghost.ID == route.From.NodeID || (route.From.NodeID == "" && ghost.ID == "(empty ID)") {
				return ghost.X + ghost.W, ghost.Y + ghost.H/2, true
			}
		}
		return trackX + trackWidth, graphTop, true
	}
	for _, port := range node.Outputs {
		if port.ID == route.From.PortID {
			return port.X, port.Y, false
		}
	}
	return node.right(), node.bottom() - 14, true
}

func (g graphLayout) destinationAnchor(route Route) (float64, float64, bool) {
	node, ok := g.NodeMap[route.To.NodeID]
	if !ok {
		for _, ghost := range g.Ghosts {
			if ghost.ID == route.To.NodeID || (route.To.NodeID == "" && ghost.ID == "(empty ID)") {
				return ghost.X, ghost.Y + ghost.H/2, true
			}
		}
		return busX, graphTop, true
	}
	for _, port := range node.Inputs {
		if port.ID == route.To.PortID {
			return port.X, port.Y, false
		}
	}
	return node.X, node.bottom() - 14, true
}

func routePath(x1, y1, x2, y2 float64, routeIndex int) (string, float64, float64) {
	if x2 > x1+50 {
		dx := math.Max(70, (x2-x1)/2)
		cp1x, cp1y := x1+dx, y1
		cp2x, cp2y := x2-dx, y2
		path := fmt.Sprintf("M %.1f %.1f C %.1f %.1f, %.1f %.1f, %.1f %.1f",
			x1, y1, cp1x, cp1y, cp2x, cp2y, x2, y2)
		lx, ly := cubicPoint(x1, y1, cp1x, cp1y, cp2x, cp2y, x2, y2, 0.22)
		return path, lx, ly
	}
	loopX := math.Max(x1, x2) + 74 + float64(routeIndex%5)*16
	cp1x, cp1y := loopX, y1
	cp2x, cp2y := loopX, y2
	path := fmt.Sprintf("M %.1f %.1f C %.1f %.1f, %.1f %.1f, %.1f %.1f",
		x1, y1, cp1x, cp1y, cp2x, cp2y, x2, y2)
	lx, ly := cubicPoint(x1, y1, cp1x, cp1y, cp2x, cp2y, x2, y2, 0.42)
	return path, lx, ly
}

func cubicPoint(x0, y0, x1, y1, x2, y2, x3, y3, t float64) (float64, float64) {
	u := 1 - t
	x := u*u*u*x0 + 3*u*u*t*x1 + 3*u*t*t*x2 + t*t*t*x3
	y := u*u*u*y0 + 3*u*u*t*y1 + 3*u*t*t*y2 + t*t*t*y3
	return x, y
}

func routeKindLabel(kind RouteKind) string {
	switch kind {
	case TrackPatch:
		return "track patch"
	case Send:
		return "send"
	default:
		if strings.TrimSpace(string(kind)) == "" {
			return "unknown"
		}
		return string(kind)
	}
}
