package mirror

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testVisitor struct {
}

var visited string

func accept(n *node, _ *properties) error {
	visited += n.desc
	return nil
}

func TestAddToGraph(t *testing.T) {
	var root *node
	add(&root, &node{order: 4, desc: "E", accept: accept})
	require.NotNil(t, root)
	add(&root, &node{order: 3, desc: "D", accept: accept})
	visit(root, &predTable{}, &properties{})
	assert.Equal(t, "DE", visited)
}

func TestInOrderTraversal(t *testing.T) {
	letters := "ABCDEFGHIJ"
	tt := []struct {
		set []int
		err error
	}{
		{set: []int{9, 1, 2, 4, 5, 6, 8, 7, 0, 3}},
		{set: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{set: []int{9, 8, 7, 6, 5, 4, 3, 2, 1, 0}},
		{set: []int{5, 3, 9, 4, 2, 1, 7, 0, 8, 6}},
	}
	for run, tstr := range tt {
		visited = ""
		var root *node
		// build a tree with different elements inserted in different
		// order, if visit is called in correct order the letters will be
		// sorted
		for _, v := range tstr.set {
			add(&root, &node{order: v, desc: string(letters[v]), accept: accept})
		}
		t.Run(strconv.Itoa(run), func(st *testing.T) {
			err := visit(root, &predTable{}, &properties{})
			require.Equal(t, err, tstr.err)
			assert.Equal(t, letters, visited)

		})
	}
}

func TestCycleDetection(t *testing.T) {
	root := &node{
		order:  3,
		accept: accept,
		left: &node{
			order:  2,
			accept: accept,
		},
		right: &node{
			order:  4,
			accept: accept,
		},
	}
	root.left.left = root
	err := hasCycle(root)
	assert.Equal(t, errCycle, err)
}

func TestPredCheck(t *testing.T) {

	nodes := []node{
		node{order: 0, desc: "A", accept: accept},
		node{order: 1, desc: "B", accept: accept},
		node{order: 2, desc: "C", accept: accept},
		node{order: 3, desc: "D", accept: accept},
		node{order: 4, desc: "E", accept: accept},
		node{order: 5, desc: "F", accept: accept},
	}
	var root *node
	add(&root, &nodes[0])
	nodes[1].pred = &nodes[0].order
	add(&root, &nodes[1])
	err := visit(root, &predTable{}, &properties{})
	require.Nil(t, err)
	// this should fail because 2 can't be predecessor of 1
	root = nil
	add(&root, &nodes[2])
	nodes[1].pred = &nodes[2].order
	add(&root, &nodes[1])
	err = visit(root, &predTable{}, &properties{})
	require.NotNil(t, err)
	require.Equal(t, errPredMissing, err)

	root = nil
	add(&root, &nodes[0])
	add(&root, &nodes[3])
	// node two was not added to graph
	nodes[3].pred = &nodes[2].order
	add(&root, &nodes[5])
	err = visit(root, &predTable{}, &properties{})
	require.NotNil(t, err)
	require.Equal(t, errPredMissing, err)
}

func TestGetPropString(t *testing.T) {
	p := properties{}
	p["key"] = "val"
	s, err := p.getString("key")
	require.Nil(t, err)
	assert.Equal(t, "val", s)
	p["key"] = 23
	_, err = p.getString("key")
	require.NotNil(t, err)
	_, err = p.getString("unknown")
	require.NotNil(t, err)
}
