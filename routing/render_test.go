package routing

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderHTMLContainsExplicitPortMapping(t *testing.T) {
	workspace := demoWorkspace()
	var output bytes.Buffer
	if err := RenderHTML(&output, workspace); err != nil {
		t.Fatalf("RenderHTML() = %v", err)
	}
	page := output.String()
	for _, expected := range []string{
		"<!doctype html>",
		"track:lead-a",
		"track:lead-b",
		"main-left",
		"bus:fx-return",
		"return-l",
		`data-kind="track_patch"`,
		`data-kind="send"`,
		"patch-b-left",
		"send-a-fx-left",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("rendered page does not contain %q", expected)
		}
	}
	if strings.Count(page, "Lead Vox") < 2 {
		t.Fatal("both same-named tracks must remain visible")
	}
}

func TestRenderHTMLShowsMissingEndpointsAndCycles(t *testing.T) {
	workspace := demoWorkspace()
	workspace.Routes = append(workspace.Routes, Route{
		ID:   "will-not-resolve",
		Kind: Send,
		From: Endpoint{NodeID: "track:missing", PortID: "out"},
		To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
	})
	var output bytes.Buffer
	if err := RenderHTML(&output, workspace); err != nil {
		t.Fatalf("RenderHTML() = %v", err)
	}
	page := output.String()
	for _, expected := range []string{"MISSING NODE", "track:missing", "missing_node", "edge-invalid"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("invalid graph rendering does not contain %q", expected)
		}
	}
}

func TestRenderSVGIsStandalone(t *testing.T) {
	var output bytes.Buffer
	if err := RenderSVG(&output, demoWorkspace()); err != nil {
		t.Fatalf("RenderSVG() = %v", err)
	}
	svg := output.String()
	if !strings.HasPrefix(svg, `<?xml version="1.0"`) {
		t.Fatal("SVG is missing XML declaration")
	}
	if !strings.Contains(svg, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Fatal("SVG is missing namespace")
	}
}

func TestRenderEscapesDisplayText(t *testing.T) {
	workspace := demoWorkspace()
	workspace.Tracks[0].Name = `<script>alert("x")</script>`
	var output bytes.Buffer
	if err := RenderHTML(&output, workspace); err != nil {
		t.Fatalf("RenderHTML() = %v", err)
	}
	if strings.Contains(output.String(), `<script>alert("x")</script>`) {
		t.Fatal("track name was injected into HTML")
	}
	if !strings.Contains(output.String(), "&lt;script&gt;") {
		t.Fatal("escaped track name not found")
	}
}
