package main

import (
	"context"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/storage"
)

func graphID(kind, id string) string { return kind + ":" + id }

func (s *apiServer) dependencyGraph(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk discovery unavailable"})
		return
	}
	nodes := make(map[string]model.GraphNode)
	edges := make(map[string]model.GraphEdge)
	addNode := func(kind, id, label string, status model.HealthState) string {
		key := graphID(kind, id)
		nodes[key] = model.GraphNode{ID: key, Type: kind, Label: label, Status: status}
		return key
	}
	addEdge := func(from, to, relationship string) {
		key := from + "\x00" + to + "\x00" + relationship
		edges[key] = model.GraphEdge{From: from, To: to, Relationship: relationship}
	}
	for _, disk := range disks {
		addNode("disk", disk.ID, disk.Name, disk.Health)
	}
	pools := storage.DiscoverPools(ctx, disks, nil)
	for _, pool := range pools {
		poolNode := addNode("pool", pool.ID, pool.Name, pool.Status)
		for _, member := range pool.Members {
			if _, ok := nodes[graphID("disk", member.DiskID)]; ok {
				addEdge(graphID("disk", member.DiskID), poolNode, "member of")
			}
		}
	}
	shares, shareErr := s.shareStore().Load()
	if shareErr == nil {
		for _, share := range shares {
			status := model.Healthy
			if !share.Enabled {
				status = model.Attention
			}
			shareNode := addNode("share", share.ID, share.Name, status)
			for _, pool := range pools {
				if pathWithin(pool.MountPath, share.Path) {
					addEdge(graphID("pool", pool.ID), shareNode, "stores")
				}
			}
			for _, disk := range disks {
				if disk.CurrentPath != "" && pathWithin(filepath.Dir(disk.CurrentPath), share.Path) {
					addEdge(graphID("disk", disk.ID), shareNode, "contains")
				}
			}
		}
	}
	stackValues, stackErr := s.dockerService.Stacks(ctx)
	if stackErr == nil {
		for _, stack := range stackValues {
			status := model.HealthState(stack.Status)
			if status == "" {
				status = model.Attention
			}
			stackNode := addNode("stack", stack.ID, stack.Name, status)
			for _, mapping := range stack.Storage {
				for _, kind := range []string{"pool", "share", "disk"} {
					if _, ok := nodes[graphID(kind, mapping.ResourceID)]; ok {
						addEdge(graphID(kind, mapping.ResourceID), stackNode, "mounted by")
					}
				}
			}
		}
	}
	if destinations, destinationErr := s.store.ListBackupDestinations(); destinationErr == nil {
		recoveryNode := addNode("recovery", "recovery", "Recovery bundles", model.Healthy)
		for _, destination := range destinations {
			destinationNode := addNode("backup", destination.ID, destination.Name, model.Healthy)
			addEdge(destinationNode, recoveryNode, "stores")
		}
	}
	result := model.DependencyGraph{GeneratedAt: time.Now().UTC(), Nodes: make([]model.GraphNode, 0, len(nodes)), Edges: make([]model.GraphEdge, 0, len(edges))}
	for _, node := range nodes {
		result.Nodes = append(result.Nodes, node)
	}
	for _, edge := range edges {
		result.Edges = append(result.Edges, edge)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].ID < result.Nodes[j].ID })
	sort.Slice(result.Edges, func(i, j int) bool {
		return result.Edges[i].From+result.Edges[i].To < result.Edges[j].From+result.Edges[j].To
	})
	writeJSON(w, http.StatusOK, result)
}

func pathWithin(root, candidate string) bool {
	root, candidate = filepath.Clean(root), filepath.Clean(candidate)
	if root == "." || candidate == "." {
		return false
	}
	return candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator))
}
