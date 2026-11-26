package routing

import (
	"fmt"
	"html"
	"io"
	"strings"
)

// RenderHTML writes a self-contained routing review page. Invalid workspaces
// are still rendered so missing endpoints and cycles can be inspected.
func RenderHTML(w io.Writer, workspace Workspace) error {
	issues := workspace.Diagnostics()
	graph := buildGraph(workspace, issues)
	issueByRoute := make(map[string][]Issue)
	cycleCount := 0
	for _, issue := range issues {
		if issue.RouteID != "" {
			issueByRoute[issue.RouteID] = append(issueByRoute[issue.RouteID], issue)
		}
		if issue.Code == IssueCycle {
			cycleCount++
		}
	}

	var page strings.Builder
	page.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	page.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	page.WriteString("<title>Audio routing map</title><style>")
	page.WriteString(pageCSS)
	page.WriteString("</style></head><body><main class=\"shell\">")
	writeMasthead(&page, workspace, len(issues), cycleCount)
	writeToolbar(&page)
	page.WriteString(`<section class="card graph-card"><div class="graph-scroll">`)
	writeGraphSVG(&page, graph, workspace)
	page.WriteString(`</div></section>`)
	writeIssues(&page, issues)
	writeRouteTable(&page, workspace, issueByRoute)
	page.WriteString(`<p class="footer">Identity is resolved by stable node/port IDs. Display names never participate in routing.</p>`)
	page.WriteString(`</main><script>`)
	page.WriteString(pageJS)
	page.WriteString(`</script></body></html>`)

	if _, err := io.WriteString(w, page.String()); err != nil {
		return fmt.Errorf("write HTML: %w", err)
	}
	return nil
}

// RenderSVG writes only the SVG graph for embedding in another document.
func RenderSVG(w io.Writer, workspace Workspace) error {
	issues := workspace.Diagnostics()
	graph := buildGraph(workspace, issues)
	var svg strings.Builder
	svg.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	writeGraphSVG(&svg, graph, workspace)
	if _, err := io.WriteString(w, svg.String()); err != nil {
		return fmt.Errorf("write SVG: %w", err)
	}
	return nil
}

func writeMasthead(builder *strings.Builder, workspace Workspace, issueCount, cycleCount int) {
	badClass := ""
	if issueCount > 0 {
		badClass = " bad"
	}
	builder.WriteString(`<header class="masthead"><div><p class="eyebrow">Signal graph / patch review</p>`)
	builder.WriteString(`<h1>Track, bus &amp; send map</h1>`)
	builder.WriteString(`<p class="lead">显式端口到端口映射。相近轨道名不会参与解析；左侧为轨道输出，右侧为 output bus 输入与输出。</p></div>`)
	builder.WriteString(`<div class="summary-grid">`)
	fmt.Fprintf(builder, `<div class="metric"><strong>%d</strong><span>tracks</span></div>`, len(workspace.Tracks))
	fmt.Fprintf(builder, `<div class="metric"><strong>%d</strong><span>output buses</span></div>`, len(workspace.Buses))
	fmt.Fprintf(builder, `<div class="metric"><strong>%d</strong><span>routes</span></div>`, len(workspace.Routes))
	fmt.Fprintf(builder, `<div class="metric%s"><strong>%d</strong><span>issues / %d cycles</span></div>`, badClass, issueCount, cycleCount)
	builder.WriteString(`</div></header>`)
}

func writeToolbar(builder *strings.Builder) {
	builder.WriteString(`<div class="toolbar"><label class="search"><input id="route-search" type="search" placeholder="搜索轨道、端口或 route ID…" autocomplete="off"></label>`)
	builder.WriteString(`<button class="chip active" data-filter="all" type="button">全部</button>`)
	builder.WriteString(`<button class="chip" data-filter="track_patch" type="button">Track patch</button>`)
	builder.WriteString(`<button class="chip" data-filter="send" type="button">Send</button>`)
	builder.WriteString(`<div class="legend"><span><i class="swatch"></i>track patch</span><span><i class="swatch send"></i>send</span><span><i class="swatch bad"></i>invalid / cycle</span></div></div>`)
}

func writeIssues(builder *strings.Builder, issues []Issue) {
	builder.WriteString(`<div class="section-title"><h2>Diagnostics</h2><span>`)
	fmt.Fprintf(builder, "%d issue(s)", len(issues))
	builder.WriteString(`</span></div>`)
	if len(issues) == 0 {
		builder.WriteString(`<div class="ok-panel">所有端点均存在，且路由图无环路。</div>`)
		return
	}
	builder.WriteString(`<section class="card issue-list">`)
	for _, issue := range issues {
		builder.WriteString(`<div class="issue"><span class="issue-code">`)
		builder.WriteString(html.EscapeString(string(issue.Code)))
		builder.WriteString(`</span><span class="issue-message">`)
		builder.WriteString(html.EscapeString(issue.Message))
		builder.WriteString(`</span>`)
		if issue.OperationIndex > 0 {
			fmt.Fprintf(builder, `<span class="issue-op">op %d</span>`, issue.OperationIndex)
		} else {
			builder.WriteString(`<span class="issue-op">workspace</span>`)
		}
		builder.WriteString(`</div>`)
	}
	builder.WriteString(`</section>`)
}

func writeRouteTable(builder *strings.Builder, workspace Workspace, issueByRoute map[string][]Issue) {
	builder.WriteString(`<div class="section-title"><h2>Route mapping</h2><span>port → port</span></div>`)
	builder.WriteString(`<section class="card table-card"><table><thead><tr><th>Route</th><th>Kind</th><th>Source</th><th>Destination</th><th>Status</th></tr></thead><tbody>`)
	if len(workspace.Routes) == 0 {
		builder.WriteString(`<tr><td colspan="5" class="empty">No routes.</td></tr>`)
	}
	for i, route := range workspace.Routes {
		issues := issueByRoute[route.ID]
		kindClass := ""
		if route.Kind == Send {
			kindClass = " send"
		}
		search := strings.ToLower(route.ID + " " + string(route.Kind) + " " +
			endpointSearch(route.From) + " " + endpointSearch(route.To) + " " +
			workspace.nodeLabel(route.From.NodeID) + " " + workspace.nodeLabel(route.To.NodeID))
		fmt.Fprintf(builder, `<tr data-row-index="%d" data-kind="%s" data-search="%s">`, i, html.EscapeString(string(route.Kind)), html.EscapeString(search))
		fmt.Fprintf(builder, `<td><code>%s</code></td>`, html.EscapeString(route.ID))
		fmt.Fprintf(builder, `<td><span class="kind%s">%s</span></td>`, kindClass, html.EscapeString(routeKindLabel(route.Kind)))
		fmt.Fprintf(builder, `<td>%s</td><td>%s</td>`, endpointCell(workspace, route.From, true), endpointCell(workspace, route.To, false))
		if len(issues) == 0 {
			builder.WriteString(`<td><span class="status">OK</span></td>`)
		} else {
			fmt.Fprintf(builder, `<td><span class="status bad">%s</span></td>`, html.EscapeString(issueBadge(issues)))
		}
		builder.WriteString(`</tr>`)
	}
	builder.WriteString(`</tbody></table></section>`)
}

func endpointCell(workspace Workspace, endpoint Endpoint, source bool) string {
	name := workspace.nodeLabel(endpoint.NodeID)
	if name == "" {
		name = "missing node"
	}
	var ports []Port
	if track, ok := workspace.track(endpoint.NodeID); ok && source {
		ports = track.Outputs
	} else if bus, ok := workspace.bus(endpoint.NodeID); ok {
		if source {
			ports = bus.Outputs
		} else {
			ports = bus.Inputs
		}
	}
	portText := endpoint.PortID
	exists := portExists(ports, endpoint.PortID)
	if exists {
		portText = portLabel(ports, endpoint.PortID)
	}
	var builder strings.Builder
	builder.WriteString(`<strong>`)
	builder.WriteString(html.EscapeString(name))
	builder.WriteString(`</strong><br><code>`)
	builder.WriteString(html.EscapeString(endpoint.NodeID + "/" + endpoint.PortID))
	builder.WriteString(`</code>`)
	if exists {
		builder.WriteString(` <code>· `)
		builder.WriteString(html.EscapeString(portText))
		builder.WriteString(`</code>`)
	} else {
		builder.WriteString(` <code style="color:#b91c1c">· missing port</code>`)
	}
	return builder.String()
}

func issueBadge(issues []Issue) string {
	for _, issue := range issues {
		if issue.Code == IssueCycle {
			return "CYCLE"
		}
	}
	for _, issue := range issues {
		if issue.Code == IssueMissingNode || issue.Code == IssueMissingPort {
			return "MISSING"
		}
	}
	return "INVALID"
}
