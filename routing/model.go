package routing

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// RouteKind describes how a source port reaches a bus input.
type RouteKind string

const (
	// TrackPatch is a track's primary output patch into an output bus.
	TrackPatch RouteKind = "track_patch"
	// Send is an auxiliary route from a track or bus output into a bus input.
	Send RouteKind = "send"
)

// Port is a stable, addressable channel on a track or bus.
type Port struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// Display returns the human-facing port label without making it an identity.
func (p Port) Display() string {
	if strings.TrimSpace(p.Label) != "" {
		return p.Label
	}
	return p.ID
}

// Track exposes output ports that can be patched or sent elsewhere.
type Track struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Outputs []Port `json:"outputs"`
}

// OutputBus receives routes on its inputs and can feed further buses from its
// outputs. A bus input is treated as feeding every bus output; this all-to-all
// assumption makes cycle detection conservative and safe for mixed effects.
type OutputBus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inputs  []Port `json:"inputs"`
	Outputs []Port `json:"outputs"`
}

// Endpoint always combines a stable node ID with a stable port ID. Names and
// list positions are deliberately absent so renamed or same-named tracks cannot
// silently retarget audio.
type Endpoint struct {
	NodeID string `json:"node"`
	PortID string `json:"port"`
}

func (e Endpoint) String() string {
	return e.NodeID + "/" + e.PortID
}

// Route is one explicit port-to-port connection. Route IDs are stable handles
// used by migration operations and diagnostics.
type Route struct {
	ID   string    `json:"id"`
	Kind RouteKind `json:"kind"`
	From Endpoint  `json:"from"`
	To   Endpoint  `json:"to"`
}

// Workspace is the complete patchable graph. Tracks and buses share one node
// ID namespace so an endpoint can never be ambiguous.
type Workspace struct {
	SchemaVersion int         `json:"schema_version,omitempty"`
	Tracks        []Track     `json:"tracks"`
	Buses         []OutputBus `json:"buses"`
	Routes        []Route     `json:"routes"`
}

func (w Workspace) clone() Workspace {
	out := w
	if w.Tracks != nil {
		out.Tracks = make([]Track, len(w.Tracks))
	}
	for i, track := range w.Tracks {
		out.Tracks[i] = track
		out.Tracks[i].Outputs = clonePorts(track.Outputs)
	}
	if w.Buses != nil {
		out.Buses = make([]OutputBus, len(w.Buses))
	}
	for i, bus := range w.Buses {
		out.Buses[i] = bus
		out.Buses[i].Inputs = clonePorts(bus.Inputs)
		out.Buses[i].Outputs = clonePorts(bus.Outputs)
	}
	if w.Routes != nil {
		out.Routes = make([]Route, len(w.Routes))
		copy(out.Routes, w.Routes)
	}
	return out
}

func clonePorts(ports []Port) []Port {
	if ports == nil {
		return nil
	}
	out := make([]Port, len(ports))
	copy(out, ports)
	return out
}

func (w Workspace) track(id string) (Track, bool) {
	for _, track := range w.Tracks {
		if track.ID == id {
			return track, true
		}
	}
	return Track{}, false
}

func (w Workspace) bus(id string) (OutputBus, bool) {
	for _, bus := range w.Buses {
		if bus.ID == id {
			return bus, true
		}
	}
	return OutputBus{}, false
}

func (w Workspace) route(id string) (int, bool) {
	for i, route := range w.Routes {
		if route.ID == id {
			return i, true
		}
	}
	return 0, false
}

func portExists(ports []Port, id string) bool {
	for _, port := range ports {
		if port.ID == id {
			return true
		}
	}
	return false
}

func portLabel(ports []Port, id string) string {
	for _, port := range ports {
		if port.ID == id {
			return port.Display()
		}
	}
	return id
}

func (w Workspace) nodeLabel(id string) string {
	if track, ok := w.track(id); ok {
		if strings.TrimSpace(track.Name) != "" {
			return track.Name
		}
		return track.ID
	}
	if bus, ok := w.bus(id); ok {
		if strings.TrimSpace(bus.Name) != "" {
			return bus.Name
		}
		return bus.ID
	}
	return ""
}

// LoadWorkspace reads one workspace document and rejects unknown fields.
func LoadWorkspace(r io.Reader) (Workspace, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var workspace Workspace
	if err := decoder.Decode(&workspace); err != nil {
		return Workspace{}, fmt.Errorf("decode workspace: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Workspace{}, fmt.Errorf("decode workspace: %w", err)
	}
	return workspace, nil
}

// LoadWorkspaceFile is the path-based counterpart to LoadWorkspace.
func LoadWorkspaceFile(path string) (Workspace, error) {
	file, err := os.Open(path)
	if err != nil {
		return Workspace{}, fmt.Errorf("open workspace: %w", err)
	}
	defer file.Close()
	return LoadWorkspace(file)
}

// SaveWorkspace writes indented JSON suitable for version control and review.
func SaveWorkspace(w io.Writer, workspace Workspace) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(workspace); err != nil {
		return fmt.Errorf("encode workspace: %w", err)
	}
	return nil
}
