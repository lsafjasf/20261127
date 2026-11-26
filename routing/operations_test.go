package routing

import (
	"errors"
	"reflect"
	"testing"
)

func TestValidMigrationKeepsLeftChannelOnMaster(t *testing.T) {
	workspace := demoWorkspace()
	operations := []Operation{
		{
			Op: OpReplaceRoute,
			Route: &Route{
				ID:   "patch-a-left",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-b", PortID: "main-left"},
				To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
			},
		},
	}
	updated, err := ApplyOperations(workspace, operations)
	if err != nil {
		t.Fatalf("ApplyOperations() = %v", err)
	}
	if err := updated.Validate(); err != nil {
		t.Fatalf("updated workspace is invalid: %v", err)
	}
	route := updated.Routes[0]
	if route.From != (Endpoint{NodeID: "track:lead-b", PortID: "main-left"}) {
		t.Fatalf("source = %+v", route.From)
	}
	if route.To != (Endpoint{NodeID: "bus:master", PortID: "main-l"}) {
		t.Fatalf("left channel was routed to %+v instead of master L", route.To)
	}
}

func TestMissingEndpointRejectsWholeBatch(t *testing.T) {
	input := demoWorkspace()
	before := input.clone()
	operations := []Operation{
		{
			Op: OpReplaceRoute,
			Route: &Route{
				ID:   "patch-a-left",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-b", PortID: "main-left"},
				To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
			},
		},
		{
			Op: OpAddRoute,
			Route: &Route{
				ID:   "wrong-left-after-migration",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-b", PortID: "main-l"},
				To:   Endpoint{NodeID: "bus:fx-return", PortID: "return-l"},
			},
		},
	}

	result, err := ApplyOperations(input, operations)
	if err == nil {
		t.Fatal("batch with a missing source port was accepted")
	}
	var diagnostics *DiagnosticsError
	if !errors.As(err, &diagnostics) {
		t.Fatalf("error type = %T", err)
	}
	if len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Code != IssueMissingPort {
		t.Fatalf("issues = %+v", diagnostics.Issues)
	}
	if diagnostics.Issues[0].OperationIndex != 2 {
		t.Fatalf("operation index = %d, want 2", diagnostics.Issues[0].OperationIndex)
	}
	if !reflect.DeepEqual(result, before) {
		t.Fatal("partial batch result was returned")
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("input workspace was mutated by a rejected batch")
	}
}

func TestRemoveThenBadAddDoesNotEraseExistingRoute(t *testing.T) {
	input := demoWorkspace()
	before := input.clone()
	operations := []Operation{
		{Op: OpRemoveRoute, RouteID: "patch-a-left"},
		{
			Op: OpAddRoute,
			Route: &Route{
				ID:   "replacement-typo",
				Kind: TrackPatch,
				From: Endpoint{NodeID: "track:lead-b", PortID: "main-l"},
				To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
			},
		},
	}
	result, err := ApplyOperations(input, operations)
	if err == nil {
		t.Fatal("bad replacement was accepted")
	}
	if !reflect.DeepEqual(result, before) {
		t.Fatal("removal remained applied after the replacement failed")
	}
	if _, ok := result.route("patch-a-left"); !ok {
		t.Fatal("existing left-channel patch disappeared")
	}
}

func TestCycleRejectsWholeBatch(t *testing.T) {
	workspace := Workspace{
		Buses: []OutputBus{
			{ID: "bus:a", Inputs: []Port{{ID: "in"}}, Outputs: []Port{{ID: "out"}}},
			{ID: "bus:b", Inputs: []Port{{ID: "in"}}, Outputs: []Port{{ID: "out"}}},
		},
	}
	before := workspace.clone()
	operations := []Operation{
		{
			Op: OpAddRoute,
			Route: &Route{
				ID:   "a-to-b",
				Kind: Send,
				From: Endpoint{NodeID: "bus:a", PortID: "out"},
				To:   Endpoint{NodeID: "bus:b", PortID: "in"},
			},
		},
		{
			Op: OpAddRoute,
			Route: &Route{
				ID:   "b-to-a",
				Kind: Send,
				From: Endpoint{NodeID: "bus:b", PortID: "out"},
				To:   Endpoint{NodeID: "bus:a", PortID: "in"},
			},
		},
	}
	result, err := ApplyOperations(workspace, operations)
	if err == nil {
		t.Fatal("cycle-forming batch was accepted")
	}
	if !hasIssueCode(err, IssueCycle) {
		t.Fatalf("error = %v, want cycle", err)
	}
	if !reflect.DeepEqual(result, before) {
		t.Fatal("cycle batch left partial routes behind")
	}
	if len(result.Routes) != 0 {
		t.Fatalf("routes = %+v, want none", result.Routes)
	}
}

func TestApplyMethodOnlyMutatesOnSuccess(t *testing.T) {
	workspace := demoWorkspace()
	before := workspace.clone()
	err := workspace.Apply([]Operation{{
		Op: OpAddRoute,
		Route: &Route{
			ID:   "missing-node",
			Kind: Send,
			From: Endpoint{NodeID: "track:absent", PortID: "out"},
			To:   Endpoint{NodeID: "bus:master", PortID: "main-l"},
		},
	}})
	if err == nil {
		t.Fatal("missing node was accepted")
	}
	if !reflect.DeepEqual(workspace, before) {
		t.Fatal("Apply mutated the receiver after failure")
	}
}

func TestRemoveMissingRouteRejectsBatch(t *testing.T) {
	workspace := demoWorkspace()
	before := workspace.clone()
	result, err := ApplyOperations(workspace, []Operation{
		{Op: OpRemoveRoute, RouteID: "does-not-exist"},
	})
	if err == nil {
		t.Fatal("missing route removal was accepted")
	}
	if !reflect.DeepEqual(result, before) {
		t.Fatal("workspace changed after missing route removal")
	}
}

func hasIssueCode(err error, code IssueCode) bool {
	var diagnostics *DiagnosticsError
	if !errors.As(err, &diagnostics) {
		return false
	}
	for _, issue := range diagnostics.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
