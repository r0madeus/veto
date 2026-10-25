package core

import (
	"fmt"
	"sort"
)

// Graph represents a directed acyclic graph of resources
type Graph struct {
	Nodes    map[string]ConfigItem
	Edges    map[string][]string // Adjacency list: Node -> Dependencies
	InDegree map[string]int
}

// NewGraph creates a new empty graph
func NewGraph() *Graph {
	return &Graph{
		Nodes:    make(map[string]ConfigItem),
		Edges:    make(map[string][]string),
		InDegree: make(map[string]int),
	}
}

// BuildGraph constructs the graph from a list of ConfigItems
func (g *Graph) BuildGraph(items []ConfigItem) error {
	// 1. Add all nodes
	for _, item := range items {
		if _, exists := g.Nodes[item.Name]; exists {
			return fmt.Errorf("duplicate resource name: %s", item.Name)
		}
		g.Nodes[item.Name] = item
		// Initialize empty edges/degree for safety
		if g.Edges[item.Name] == nil {
			g.Edges[item.Name] = []string{}
		}
		g.InDegree[item.Name] = 0
	}

	// 2. Add edges based on DependsOn
	for _, item := range items {
		for _, dep := range item.DependsOn {
			if _, exists := g.Nodes[dep]; !exists {
				return fmt.Errorf("resource '%s' depends on unknown resource '%s'", item.Name, dep)
			}

			// Dependency means: dep -> item (dep must run before item)
			// So Edge is from dep to item.
			g.Edges[dep] = append(g.Edges[dep], item.Name)
			g.InDegree[item.Name]++
		}
	}

	return nil
}

// TopologicalSort returns layers of items that can be executed in parallel.
// Returns an error if a cycle is detected.
func (g *Graph) TopologicalSort() ([][]ConfigItem, error) {
	// Kahn's Algorithm
	var queue []string

	// Copy InDegree map to avoid mutating the graph state permanently (if reused)
	inDegree := make(map[string]int)
	for k, v := range g.InDegree {
		inDegree[k] = v
	}

	// Find all nodes with 0 in-degree (no dependencies)
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Sort queue for deterministic initial order
	sort.Strings(queue)

	var layers [][]ConfigItem
	processedCount := 0
	totalNodes := len(g.Nodes)

	for len(queue) > 0 {
		var currentLayer []ConfigItem
		var nextQueue []string

		// Process everything currently in the queue (this forms one layer)
		// NOTE: In standard Kahn's, we pop one by one. To get parallel layers,
		// we process the WHOLE queue at this snapshot, then find new 0-degree nodes.

		layerNames := make([]string, len(queue))
		copy(layerNames, queue)

		for _, nodeName := range layerNames {
			item := g.Nodes[nodeName]
			currentLayer = append(currentLayer, item)
			processedCount++

			// Decrease in-degree of neighbors
			for _, neighbor := range g.Edges[nodeName] {
				inDegree[neighbor]--
				if inDegree[neighbor] == 0 {
					nextQueue = append(nextQueue, neighbor)
				}
			}
		}

		layers = append(layers, currentLayer)

		// Sort next layer for deterministic behavior
		sort.Strings(nextQueue)
		queue = nextQueue
	}

	if processedCount != totalNodes {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return layers, nil
}
