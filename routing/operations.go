package routing

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// OperationKind is one atomic edit inside a batch.
type OperationKind string

const (
	OpAddRoute     OperationKind = "add_route"
	OpRemoveRoute  OperationKind = "remove_route"
	OpReplaceRoute OperationKind = "replace_route"
)

// Operation edits one route. add_route and replace_route carry the complete
// route; remove_route carries only route_id.
type Operation struct {
	Op      OperationKind `json:"op"`
	Route   *Route        `json:"route,omitempty"`
	RouteID string        `json:"route_id,omitempty"`
}

// ApplyOperations applies a batch on a clone. The input workspace is never
// changed. If any operation is malformed, references a missing endpoint, or
// would introduce a cycle, application stops and the input is returned.
func ApplyOperations(input Workspace, operations []Operation) (Workspace, error) {
	staged := input.clone()
	if err := staged.Validate(); err != nil {
		return input, err
	}
	if len(operations) == 0 {
		return staged, nil
	}

	for i, operation := range operations {
		if err := applyOne(&staged, operation); err != nil {
			return input, annotateOperation(err, i+1)
		}
		if err := staged.Validate(); err != nil {
			return input, annotateOperation(err, i+1)
		}
	}
	return staged, nil
}

// Apply mutates the receiver only after the complete batch succeeds.
func (w *Workspace) Apply(operations []Operation) error {
	if w == nil {
		return fmt.Errorf("apply operations: nil workspace")
	}
	result, err := ApplyOperations(*w, operations)
	if err != nil {
		return err
	}
	*w = result
	return nil
}

func applyOne(workspace *Workspace, operation Operation) error {
	switch operation.Op {
	case OpAddRoute:
		if operation.Route == nil {
			return operationIssue(IssueInvalidOp, "add_route requires a route")
		}
		if operation.RouteID != "" {
			return operationIssue(IssueInvalidOp, "add_route must not set route_id")
		}
		workspace.Routes = append(workspace.Routes, *operation.Route)
	case OpRemoveRoute:
		if operation.Route != nil {
			return operationIssue(IssueInvalidOp, "remove_route must not set route")
		}
		if operation.RouteID == "" {
			return operationIssue(IssueInvalidOp, "remove_route requires route_id")
		}
		index, ok := workspace.route(operation.RouteID)
		if !ok {
			return operationIssueWithRoute(IssueInvalidOp, operation.RouteID,
				fmt.Sprintf("remove_route: route %q does not exist", operation.RouteID))
		}
		workspace.Routes = append(workspace.Routes[:index], workspace.Routes[index+1:]...)
	case OpReplaceRoute:
		if operation.Route == nil {
			return operationIssue(IssueInvalidOp, "replace_route requires a route")
		}
		if operation.RouteID != "" && operation.RouteID != operation.Route.ID {
			return operationIssue(IssueInvalidOp, "replace_route route_id must match route.id")
		}
		index, ok := workspace.route(operation.Route.ID)
		if !ok {
			return operationIssueWithRoute(IssueInvalidOp, operation.Route.ID,
				fmt.Sprintf("replace_route: route %q does not exist", operation.Route.ID))
		}
		workspace.Routes[index] = *operation.Route
	default:
		return operationIssue(IssueInvalidOp, fmt.Sprintf("unknown operation %q", operation.Op))
	}
	return nil
}

func operationIssue(code IssueCode, message string) error {
	return &DiagnosticsError{Issues: []Issue{{Code: code, Message: message}}}
}

func operationIssueWithRoute(code IssueCode, routeID, message string) error {
	return &DiagnosticsError{Issues: []Issue{{
		Code:    code,
		Message: message,
		RouteID: routeID,
	}}}
}

func annotateOperation(err error, index int) error {
	diagnostics, ok := err.(*DiagnosticsError)
	if !ok {
		return fmt.Errorf("operation %d: %w", index, err)
	}
	issues := make([]Issue, len(diagnostics.Issues))
	copy(issues, diagnostics.Issues)
	for i := range issues {
		issues[i].OperationIndex = index
		issues[i].Message = fmt.Sprintf("operation %d: %s", index, issues[i].Message)
	}
	return &DiagnosticsError{Issues: issues}
}

// LoadOperations reads a JSON array of route operations.
func LoadOperations(r io.Reader) ([]Operation, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var operations []Operation
	if err := decoder.Decode(&operations); err != nil {
		return nil, fmt.Errorf("decode operations: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode operations: %w", err)
	}
	return operations, nil
}

// LoadOperationsFile is the path-based counterpart to LoadOperations.
func LoadOperationsFile(path string) ([]Operation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open operations: %w", err)
	}
	defer file.Close()
	return LoadOperations(file)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("trailing JSON value")
	}
	return err
}
