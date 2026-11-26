package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// TaskFunc renders one checkpointed segment and returns its reusable artifact.
type TaskFunc func(ctx context.Context, rc *RunContext) (*Artifact, error)

// Task is a node in the rendering dependency graph.
type Task struct {
	ID        string
	DependsOn []string
	Version   string
	Inputs    any
	CacheKey  string
	Run       TaskFunc
}

type graph struct {
	tasks      map[string]Task
	indegree   map[string]int
	dependents map[string][]string
	roots      []string
}

func buildGraph(tasks []Task) (*graph, error) {
	result := &graph{
		tasks:      make(map[string]Task, len(tasks)),
		indegree:   make(map[string]int, len(tasks)),
		dependents: make(map[string][]string, len(tasks)),
	}

	for _, task := range tasks {
		if task.ID == "" {
			return nil, fmt.Errorf("%w: task id is empty", ErrDAGDuplicateNode)
		}
		if _, exists := result.tasks[task.ID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDAGDuplicateNode, task.ID)
		}
		if task.Run == nil {
			return nil, fmt.Errorf("task %s has no run function", task.ID)
		}
		result.tasks[task.ID] = task
		result.indegree[task.ID] = len(task.DependsOn)
	}

	for _, task := range tasks {
		seen := make(map[string]bool, len(task.DependsOn))
		for _, dep := range task.DependsOn {
			if dep == task.ID {
				return nil, fmt.Errorf("%w: %s depends on itself", ErrDAGCycle, task.ID)
			}
			if _, ok := result.tasks[dep]; !ok {
				return nil, fmt.Errorf("%w: task %s needs %s", ErrDAGMissingDep, task.ID, dep)
			}
			if seen[dep] {
				continue
			}
			seen[dep] = true
			result.dependents[dep] = append(result.dependents[dep], task.ID)
		}
		result.indegree[task.ID] = len(seen)
		if result.indegree[task.ID] == 0 {
			result.roots = append(result.roots, task.ID)
		}
	}

	if err := detectCycle(result); err != nil {
		return nil, err
	}
	sort.Strings(result.roots)
	for id := range result.dependents {
		sort.Strings(result.dependents[id])
	}
	return result, nil
}

func detectCycle(g *graph) error {
	indegree := make(map[string]int, len(g.indegree))
	queue := make([]string, 0, len(g.indegree))
	for id, degree := range g.indegree {
		indegree[id] = degree
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		next := append([]string(nil), g.dependents[id]...)
		sort.Strings(next)
		for _, dependent := range next {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
		sort.Strings(queue)
	}
	if processed != len(g.tasks) {
		remaining := make([]string, 0)
		for id, degree := range indegree {
			if degree > 0 {
				remaining = append(remaining, id)
			}
		}
		sort.Strings(remaining)
		return fmt.Errorf("%w: %s", ErrDAGCycle, joinIDs(remaining))
	}
	return nil
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}

func (t Task) resolvedCacheKey() (string, error) {
	if t.CacheKey != "" {
		return t.CacheKey, nil
	}

	hasher := sha256.New()
	fmt.Fprintf(hasher, "render-checkpoint/v1\nnode=%s\nversion=%s\n", t.ID, t.Version)
	if t.Inputs != nil {
		data, err := json.Marshal(t.Inputs)
		if err != nil {
			return "", fmt.Errorf("encode inputs for %s: %w", t.ID, err)
		}
		hasher.Write(data)
	}
	return "task:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func dependenciesMatch(record *NodeRecord, dependencyIDs []string, artifacts map[string]*Artifact) error {
	if record == nil || record.Status != StatusCompleted || record.Artifact == nil {
		return errors.New("node has no completed checkpoint")
	}
	if len(record.Dependencies) != len(dependencyIDs) {
		return fmt.Errorf("dependency set changed: checkpoint has %d, graph has %d", len(record.Dependencies), len(dependencyIDs))
	}
	for _, id := range dependencyIDs {
		expected, ok := record.Dependencies[id]
		if !ok {
			return fmt.Errorf("dependency %s was not checkpointed", id)
		}
		actual := artifacts[id]
		if actual == nil {
			return fmt.Errorf("%w: %s", ErrDependencyMissing, id)
		}
		if expected.Digest != actual.Digest || expected.Size != actual.Size {
			return fmt.Errorf("dependency %s changed", id)
		}
	}
	return nil
}
