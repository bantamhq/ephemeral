package tui

import (
	"sort"
	"strings"

	"ephemeral/internal/client"
)

type NodeKind int

const (
	NodeRoot NodeKind = iota
	NodeFolder
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

	Repo   *client.Repo
	Folder *client.Folder
}

func BuildTree(folders []client.Folder, repos []client.Repo) []*TreeNode {
	root := &TreeNode{
		Kind:     NodeRoot,
		Name:     "Repositories",
		ID:       "",
		Children: []*TreeNode{},
		Expanded: true,
	}

	folderMap := make(map[string]*TreeNode)

	for i := range folders {
		f := &folders[i]
		node := &TreeNode{
			Kind:     NodeFolder,
			Name:     f.Name,
			ID:       f.ID,
			ParentID: f.ParentID,
			Children: []*TreeNode{},
			Expanded: true,
			Folder:   f,
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
		root.Children = append(root.Children, node)
	}

	for _, f := range folders {
		attachNode(folderMap[f.ID], f.ParentID)
	}

	for i := range repos {
		r := &repos[i]
		node := &TreeNode{
			Kind: NodeRepo,
			Name: r.Name,
			ID:   r.ID,
			Repo: r,
		}
		attachNode(node, r.FolderID)
	}

	sortNodes(root.Children)
	for _, node := range folderMap {
		sortNodes(node.Children)
	}

	setDepths([]*TreeNode{root}, 0)

	return []*TreeNode{root}
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
		if node.Kind == NodeFolder || node.Kind == NodeRoot {
			setDepths(node.Children, depth+1)
		}
	}
}

func FlattenTree(nodes []*TreeNode) []*TreeNode {
	var result []*TreeNode
	for _, node := range nodes {
		result = append(result, node)
		if (node.Kind == NodeFolder || node.Kind == NodeRoot) && node.Expanded {
			result = append(result, FlattenTree(node.Children)...)
		}
	}
	return result
}
