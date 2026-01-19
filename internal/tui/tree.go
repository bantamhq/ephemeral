package tui

import (
	"sort"
	"strings"

	"ephemeral/internal/client"
)

type NodeKind int

const (
	NodeFolder NodeKind = iota
	NodeRepo
)

type TreeNode struct {
	Kind     NodeKind
	Name     string
	ID       string
	ParentID *string
	Children []*TreeNode
	Expanded bool
	Depth    int
}

func BuildTree(folders []client.Folder, repos []client.Repo) []*TreeNode {
	folderMap := make(map[string]*TreeNode)
	var roots []*TreeNode

	for _, f := range folders {
		node := &TreeNode{
			Kind:     NodeFolder,
			Name:     f.Name,
			ID:       f.ID,
			ParentID: f.ParentID,
			Children: []*TreeNode{},
			Expanded: true,
		}
		folderMap[f.ID] = node
	}

	attachNode := func(node *TreeNode, parentID *string) {
		if parentID != nil {
			if parent, ok := folderMap[*parentID]; ok {
				parent.Children = append(parent.Children, node)
				return
			}
		}
		roots = append(roots, node)
	}

	for _, f := range folders {
		attachNode(folderMap[f.ID], f.ParentID)
	}

	for _, r := range repos {
		node := &TreeNode{
			Kind: NodeRepo,
			Name: r.Name,
			ID:   r.ID,
		}
		attachNode(node, r.FolderID)
	}

	sortNodes(roots)
	for _, node := range folderMap {
		sortNodes(node.Children)
	}

	setDepths(roots, 0)

	return roots
}

func sortNodes(nodes []*TreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Kind != nodes[j].Kind {
			return nodes[i].Kind == NodeFolder
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
}

func setDepths(nodes []*TreeNode, depth int) {
	for _, node := range nodes {
		node.Depth = depth
		if node.Kind == NodeFolder {
			setDepths(node.Children, depth+1)
		}
	}
}

func FlattenTree(nodes []*TreeNode) []*TreeNode {
	var result []*TreeNode
	for _, node := range nodes {
		result = append(result, node)
		if node.Kind == NodeFolder && node.Expanded {
			result = append(result, FlattenTree(node.Children)...)
		}
	}
	return result
}
