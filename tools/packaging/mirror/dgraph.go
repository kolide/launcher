package mirror

import (
	"github.com/go-kit/kit/log"

	"github.com/pkg/errors"
)

var errCycle = errors.New("cycle in graph")
var errPredMissing = errors.New("missing predecessor")

type properties map[string]interface{}

func (ps properties) set(key string, val interface{}) {
	ps[key] = val
}

func (ps properties) getString(key string) (string, error) {
	if v, ok := ps[key]; ok {
		if s, ok := v.(string); ok {
			return s, nil
		}
		return "", errors.Errorf("value for %q is wrong type %+v", key, v)
	}
	return "", errors.Errorf("missing value for %q", key)
}

type predTable map[int]struct{}

func (pm predTable) markSeen(v int) {
	pm[v] = struct{}{}
}
func (pm predTable) visited(v *int) bool {
	// no predecessors always returns true
	if v == nil {
		return true
	}
	_, ok := pm[*v]
	return ok
}

type acceptor func(n *node, props *properties) error

// DAG node
type node struct {
	// pred may be an integer of an node that must be
	// visited  before this one
	pred *int
	// Defines precedence in the dependency graph.  order 0 happens before order 1.
	order int
	desc  string
	// internal, used for cycle detection
	perm bool
	// internal, used for cycle detection
	temp   bool
	logger log.Logger
	left   *node
	right  *node
	// Pointer to function that is executed when this node is visited.
	accept acceptor
}

// Add nodes to build a binary tree
func add(root **node, n *node) {
	if *root == nil {
		*root = n
		return
	}
	if n.order <= (*root).order {
		add(&(*root).left, n)
		return
	}
	if n.order > (*root).order {
		add(&(*root).right, n)
		return
	}
}

// Check to see if tree is acyclic
func hasCycle(n *node) error {
	if n == nil {
		return nil
	}
	if n.perm {
		return nil
	}
	if n.temp {
		return errCycle
	}
	n.temp = true
	if n.left != nil {
		if err := hasCycle(n.left); err != nil {
			return err
		}
	}
	if n.right != nil {
		if err := hasCycle(n.right); err != nil {
			return err
		}
	}
	n.perm = true
	return nil
}

// Do an in order traversal of DAG, executing each node's acceptor function
// if predecessor rules have been satisfied.
func visit(n *node, preds *predTable, props *properties) error {
	// no children we're done
	if n == nil {
		return nil
	}
	// recursively visit left children
	if err := visit(n.left, preds, props); err != nil {
		return err
	}
	// make sure prerequisites have been seen already
	if !preds.visited(n.pred) {
		return errPredMissing
	}
	// execute this in order
	if err := n.accept(n, props); err != nil {
		return err
	}
	// indicate that this operation is complete so successors that depend on
	// this will run
	preds.markSeen(n.order)
	// recursively visit right children
	if err := visit(n.right, preds, props); err != nil {
		return err
	}
	return nil
}
