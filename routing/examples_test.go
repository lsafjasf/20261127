package routing

import (
	"reflect"
	"testing"
)

func TestExampleMigration(t *testing.T) {
	workspace, err := LoadWorkspaceFile("../examples/workspace.json")
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if err := workspace.Validate(); err != nil {
		t.Fatalf("example workspace is invalid: %v", err)
	}

	operations, err := LoadOperationsFile("../examples/migrate-lead-b.json")
	if err != nil {
		t.Fatalf("load migration: %v", err)
	}
	updated, err := ApplyOperations(workspace, operations)
	if err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if err := updated.Validate(); err != nil {
		t.Fatalf("migrated workspace is invalid: %v", err)
	}
	route := updated.Routes[0]
	if route.From != (Endpoint{NodeID: "track:lead-b", PortID: "main-left"}) {
		t.Fatalf("migrated source = %+v", route.From)
	}
	if route.To != (Endpoint{NodeID: "bus:master", PortID: "main-l"}) {
		t.Fatalf("left channel migrated to %+v, want master L", route.To)
	}
}

func TestExampleBadBatchIsRejected(t *testing.T) {
	workspace, err := LoadWorkspaceFile("../examples/workspace.json")
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	operations, err := LoadOperationsFile("../examples/reject-missing-port.json")
	if err != nil {
		t.Fatalf("load bad batch: %v", err)
	}
	before := workspace.clone()
	result, err := ApplyOperations(workspace, operations)
	if err == nil {
		t.Fatal("bad example batch was accepted")
	}
	if !hasIssueCode(err, IssueMissingPort) {
		t.Fatalf("error = %v, want missing port", err)
	}
	if !reflect.DeepEqual(result, before) {
		t.Fatal("bad example left a partial migration")
	}
}

func TestInvalidExampleReportsBothProblemKinds(t *testing.T) {
	workspace, err := LoadWorkspaceFile("../examples/invalid-workspace.json")
	if err != nil {
		t.Fatalf("load invalid workspace: %v", err)
	}
	issues := workspace.Diagnostics()
	if !hasIssue(issues, IssueMissingPort, "missing-bus-port") {
		t.Fatalf("missing port not reported: %+v", issues)
	}
	var cycleSeen bool
	for _, issue := range issues {
		if issue.Code == IssueCycle {
			cycleSeen = true
		}
	}
	if !cycleSeen {
		t.Fatalf("cycle not reported: %+v", issues)
	}
}
