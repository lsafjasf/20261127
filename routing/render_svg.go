package routing

import (
	"fmt"
	"html"
	"strings"
)

func writeGraphSVG(builder *strings.Builder, graph graphLayout, workspace Workspace) {
	fmt.Fprintf(builder, `<svg id="routing-map" class="routing-map" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" role="img" aria-label="Track patch, output bus and send routing map">`, graph.Width, graph.Height)
	builder.WriteString(`<style>`)
	builder.WriteString(svgCSS)
	builder.WriteString(`</style>`)
	writeSVGDefs(builder)
	writeEdges(builder, graph, workspace)
	for _, node := range graph.Tracks {
		writeNode(builder, node)
	}
	for _, node := range graph.Buses {
		writeNode(builder, node)
	}
	for _, ghost := range graph.Ghosts {
		writeGhost(builder, ghost)
	}
	builder.WriteString(`</svg>`)
}

func writeSVGDefs(builder *strings.Builder) {
	builder.WriteString(`<defs>
<pattern id="dot-grid" width="24" height="24" patternUnits="userSpaceOnUse"><circle cx="1" cy="1" r="1" fill="#d9e0e8"/></pattern>
<filter id="node-shadow" x="-20%" y="-20%" width="140%" height="150%"><feDropShadow dx="0" dy="5" stdDeviation="7" flood-color="#17324d" flood-opacity="0.10"/></filter>
`)
	for _, marker := range []struct {
		id    string
		color string
	}{
		{"arrow-patch", "#2563eb"},
		{"arrow-send", "#c56a0b"},
		{"arrow-error", "#dc2626"},
	} {
		fmt.Fprintf(builder, `<marker id="%s" viewBox="0 0 10 10" refX="8.5" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="%s"/></marker>`, marker.id, marker.color)
	}
	builder.WriteString("</defs>\n")
}

func writeEdges(builder *strings.Builder, graph graphLayout, workspace Workspace) {
	builder.WriteString(`<g class="edges" fill="none">`)
	for _, route := range graph.Routes {
		kind := string(route.Route.Kind)
		if kind == "" {
			kind = "unknown"
		}
		class := "route-edge"
		marker := "arrow-patch"
		stroke := "#2563eb"
		dash := ""
		if route.Route.Kind == Send {
			marker = "arrow-send"
			stroke = "#c56a0b"
			dash = ` stroke-dasharray="7 5"`
		}
		if route.Invalid || route.Cycle {
			class += " edge-invalid"
			marker = "arrow-error"
			stroke = "#dc2626"
			dash = ` stroke-dasharray="3 4"`
		}
		if route.Cycle {
			class += " edge-cycle"
			stroke = "#be123c"
			dash = ` stroke-dasharray="2 5"`
		}
		search := strings.ToLower(route.Route.ID + " " + kind + " " + endpointSearch(route.Route.From) + " " + endpointSearch(route.Route.To))
		fmt.Fprintf(builder, `<g class="%s" data-route-index="%d" data-kind="%s" data-from-node="%s" data-to-node="%s" data-search="%s">`,
			class, route.Index, html.EscapeString(kind), html.EscapeString(route.Route.From.NodeID),
			html.EscapeString(route.Route.To.NodeID), html.EscapeString(search))
		fmt.Fprintf(builder, `<path d="%s" class="edge-line" fill="none" stroke="%s" stroke-width="2.2"%s marker-end="url(#%s)"/>`,
			route.Path, stroke, dash, marker)
		fmt.Fprintf(builder, `<circle cx="%.1f" cy="%.1f" r="3.2" class="edge-source" fill="%s"/>`,
			route.FromX, route.FromY, stroke)
		label := route.Route.ID
		if label == "" {
			label = "(no route ID)"
		}
		if len([]rune(label)) > 30 {
			label = string([]rune(label)[:27]) + "..."
		}
		fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="edge-label" text-anchor="middle">%s</text>`,
			route.LabelX, route.LabelY-5, html.EscapeString(label))
		fmt.Fprintf(builder, `<title>%s %s → %s</title></g>`,
			html.EscapeString(routeKindLabel(route.Route.Kind)),
			html.EscapeString(workspaceEndpointLabel(workspace, route.Route.From)),
			html.EscapeString(workspaceEndpointLabel(workspace, route.Route.To)))
	}
	builder.WriteString(`</g>`)
}

func writeNode(builder *strings.Builder, node nodeLayout) {
	kindClass := "node-track"
	badge := "TRACK"
	if node.Kind == nodeBus {
		kindClass = "node-bus"
		badge = "OUTPUT BUS"
	}
	search := strings.ToLower(node.ID + " " + node.Name + " " + badge)
	fmt.Fprintf(builder, `<g class="node %s" data-node-id="%s" data-search="%s">`,
		kindClass, html.EscapeString(node.ID), html.EscapeString(search))
	fmt.Fprintf(builder, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="12" class="node-box"/>`,
		node.X, node.Y, node.W, node.H)
	fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="node-badge">%s</text>`,
		node.X+16, node.Y+20, badge)
	fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="node-name">%s</text>`,
		node.X+16, node.Y+41, html.EscapeString(truncate(node.Name, 30)))
	fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="node-id">%s</text>`,
		node.X+16, node.Y+55, html.EscapeString(truncate(node.ID, 34)))
	fmt.Fprintf(builder, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="node-divider"/>`,
		node.X, node.Y+nodeHeader-4, node.right(), node.Y+nodeHeader-4)

	for _, port := range node.Inputs {
		fmt.Fprintf(builder, `<circle cx="%.1f" cy="%.1f" r="4.5" class="port port-input"/>`, port.X, port.Y)
		fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="port-label" text-anchor="start">%s</text>`,
			port.X+14, port.Y+4, html.EscapeString(truncate(port.Text, 28)))
	}
	for _, port := range node.Outputs {
		fmt.Fprintf(builder, `<circle cx="%.1f" cy="%.1f" r="4.5" class="port port-output"/>`, port.X, port.Y)
		fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="port-label" text-anchor="end">%s</text>`,
			port.X-14, port.Y+4, html.EscapeString(truncate(port.Text, 28)))
	}
	if len(node.Inputs) > 1 && len(node.Outputs) > 1 {
		fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="port-direction" text-anchor="middle">IN → OUT</text>`,
			node.X+node.W/2, node.Y+nodeHeader+10)
	}
	fmt.Fprintf(builder, `<title>%s %s (%s)</title></g>`, badge, html.EscapeString(node.Name), html.EscapeString(node.ID))
}

func writeGhost(builder *strings.Builder, ghost ghostLayout) {
	fmt.Fprintf(builder, `<g class="ghost-node" data-node-id="%s">`, html.EscapeString(ghost.ID))
	fmt.Fprintf(builder, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="10" class="ghost-box"/>`,
		ghost.X, ghost.Y, ghost.W, ghost.H)
	fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="ghost-title">MISSING NODE</text>`,
		ghost.X+16, ghost.Y+24)
	fmt.Fprintf(builder, `<text x="%.1f" y="%.1f" class="ghost-id">%s</text>`,
		ghost.X+16, ghost.Y+44, html.EscapeString(truncate(ghost.ID, 34)))
	fmt.Fprintf(builder, `<title>Unresolved endpoint node: %s</title></g>`, html.EscapeString(ghost.ID))
}

func workspaceEndpointLabel(workspace Workspace, endpoint Endpoint) string {
	nodeLabel := workspace.nodeLabel(endpoint.NodeID)
	if nodeLabel == "" {
		return endpoint.String() + " (missing node)"
	}
	var ports []Port
	if track, ok := workspace.track(endpoint.NodeID); ok {
		ports = track.Outputs
	} else if bus, ok := workspace.bus(endpoint.NodeID); ok {
		ports = append(append([]Port(nil), bus.Inputs...), bus.Outputs...)
	}
	label := endpoint.PortID
	if portExists(ports, endpoint.PortID) {
		label = portLabel(ports, endpoint.PortID)
	} else {
		label += " (missing port)"
	}
	return nodeLabel + " / " + label
}

func endpointSearch(endpoint Endpoint) string {
	return endpoint.NodeID + " " + endpoint.PortID
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit < 4 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
