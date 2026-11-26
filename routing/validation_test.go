package routing

import (
	"errors"
	"strings"
	"testing"
)

func demoWorkspace() Workspace {
	return Workspace{
		SchemaVersion: 1,
		Tracks: []Track{
			{
				ID:   "track:lead-a",
				Name: "Lead Vox",
				Outputs: []Port{
					{ID: "main-l", Label: "Main L"},
					{ID: "main-r", Label: "Main R"},
				},
			},
			{
				ID:   "track:lead-b",
				Name: "Lead Vox",
				Outputs: []Port{
					{ID: "main-left", Label: "Main L"},
					{ID: "main-right", Label: "Main R"},
				},
			},
		},
		Buses: []OutputBus{
			{
				ID:      "bus:master",
				Name:    "Main Output",
				Inputs:  []Port{{ID: "main-l", Label: "L"}, {ID: "main-r", Label: "R"}},
				Outputs: []Port{{ID: "program-l", Label: "L"}, {ID: "program-r", Label: "R"}},
			},
			{
				ID:     "bus:fx-return",
				Name:   "FX Return",
				Inputs: []Port{{ID: "return-l", Label: "L"}, {ID: "return-r", Label: "R"}},
			},
		},
		Routes: []Route{
			{
				ID:   "patch-a-left",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-a", PortID: "main-l"},
				To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
			},
			{
				ID:   "patch-a-right",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-a", PortID: "main-r"},
				To:   Endpoint{NodeID: "bus:master", PortID: "main-r"},
			},
			{
				ID:   "patch-b-left",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-b", PortID: "main-left"},
				To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
			},
			{
				ID:   "send-a-fx-left",
				Kind: Send,
				From: Endpoint{NodeID: "track:lead-a", PortID: "main-l"},
				To:   Endpoint{NodeID: "bus:fx-return", PortID: "return-l"},
			},
		},
	}
}

func TestDemoWorkspaceIsValid(t *testing.T) {
	workspace := demoWorkspace()
	if err := workspace.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestSimilarNamesUseStablePortIDs(t *testing.T) {
	workspace := demoWorkspace()
	workspace.Tracks[0].Name = "Renamed Vocal"
	workspace.Tracks[1].Name = "Renamed Vocal"
	if err := workspace.Validate(); err != nil {
		t.Fatalf("renamed tracks must not affect route resolution: %v", err)
	}

	route := workspace.Routes[2]
	if route.From.NodeID != "track:lead-b" || route.From.PortID != "main-left" {
		t.Fatalf("route was retargeted without an explicit ID change: %+v", route.From)
	}
	if route.To.NodeID != "bus:master" || route.To.PortID != "main-l" {
		t.Fatalf("left channel no longer targets master L: %+v", route.To)
	}
}

func TestDiagnosticsReportMissingNodeAndPort(t *testing.T) {
	workspace := demoWorkspace()
	workspace.Routes = append(workspace.Routes,
		Route{
			ID:   "missing-source-node",
			Kind: Send,
			From: Endpoint{NodeID: "track:ghost", PortID: "main-l"},
			To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
		},
		Route{
			ID:   "missing-destination-port",
			Kind: Send,
			From: Endpoint{NodeID: "track:lead-a", PortID: "main-l"},
			To:   Endpoint{NodeID: "bus:fx-return", PortID: "return-x"},
		},
	)

	issues := workspace.Diagnostics()
	if !hasIssue(issues, IssueMissingNode, "missing-source-node") {
		t.Fatalf("missing source node not reported: %+v", issues)
	}
	if !hasIssue(issues, IssueMissingPort, "missing-destination-port") {
		t.Fatalf("missing destination port not reported: %+v", issues)
	}
	if !strings.Contains(issues[len(issues)-1].Message, "return-x") {
		t.Fatalf("diagnostic must name the missing port, got %q", issues[len(issues)-1].Message)
	}
}

func TestTrackPatchCannotStartAtBus(t *testing.T) {
	workspace := demoWorkspace()
	workspace.Routes = append(workspace.Routes, Route{
		ID:   "bad-patch-source",
		Kind: TrackPatch,
		From: Endpoint{NodeID: "bus:master", PortID: "program-l"},
		To:   Endpoint{NodeID: "bus:fx-return", PortID: "return-l"},
	})
	if !hasIssue(workspace.Diagnostics(), IssueInvalidEndpoint, "bad-patch-source") {
		t.Fatal("track_patch from a bus output was accepted")
	}
}

func TestCycleDiagnosticIncludesRoutePath(t *testing.T) {
	workspace := Workspace{
		Buses: []OutputBus{
			{
				ID:      "bus:a",
				Inputs:  []Port{{ID: "in"}},
				Outputs: []Port{{ID: "out"}},
			},
			{
				ID:      "bus:b",
				Inputs:  []Port{{ID: "in"}},
				Outputs: []Port{{ID: "out"}},
			},
		},
		Routes: []Route{
			{
				ID:   "a-to-b",
				Kind: Send,
				From: Endpoint{NodeID: "bus:a", PortID: "out"},
				To:   Endpoint{NodeID: "bus:b", PortID: "in"},
			},
			{
				ID:   "b-to-a",
				Kind: Send,
				From: Endpoint{NodeID: "bus:b", PortID: "out"},
				To:   Endpoint{NodeID: "bus:a", PortID: "in"},
			},
		},
	}

	diagnostics := workspace.Diagnostics()
	var cycle *Issue
	for i := range diagnostics {
		if diagnostics[i].Code == IssueCycle {
			cycle = &diagnostics[i]
		}
	}
	if cycle == nil {
		t.Fatalf("cycle not reported: %+v", diagnostics)
	}
	if strings.Join(cycle.RouteIDs, ",") != "a-to-b,b-to-a" && strings.Join(cycle.RouteIDs, ",") != "b-to-a,a-to-b" {
		t.Fatalf("cycle routes = %v", cycle.RouteIDs)
	}
	if len(cycle.Cycle) != 3 {
		t.Fatalf("cycle path = %v, want closed path with three nodes", cycle.Cycle)
	}
}

func TestValidateReturnsDiagnosticsError(t *testing.T) {
	workspace := demoWorkspace()
	workspace.Routes[0].From.PortID = "left-typo"
	err := workspace.Validate()
	var diagnostics *DiagnosticsError
	if !errors.As(err, &diagnostics) {
		t.Fatalf("Validate() error type = %T, want *DiagnosticsError", err)
	}
	if len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Code != IssueMissingPort {
		t.Fatalf("unexpected issues: %+v", diagnostics.Issues)
	}
}

func hasIssue(issues []Issue, code IssueCode, routeID string) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.RouteID == routeID {
			return true
		}
	}
	return false
}
