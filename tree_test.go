package onet

import (
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hy06ix/onet/log"
	"github.com/hy06ix/onet/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/key"
)

var prefix = "127.0.0.1:"

func TestSubset(t *testing.T) {
	// subset of 10 from roster of 1: degenerate case
	names := genLocalhostPeerNames(1, 0)
	ro := genRoster(tSuite, names)
	r := ro.RandomSubset(ro.List[0], 10)
	// (return just the root)
	assert.Equal(t, len(r.List), 1)
	assert.Equal(t, r.List[0], ro.List[0])
	assert.NotContains(t, r.List[1:], ro.List[0])

	// subset of 4 from a roster of 20: all returned should be in orig
	// roster.
	names = genLocalhostPeerNames(20, 0)
	ro = genRoster(tSuite, names)
	r = ro.RandomSubset(ro.List[0], 4)
	assert.Equal(t, len(r.List), 5)
	for _, x := range r.List {
		assert.Contains(t, ro.List, x)
	}
	assert.NotContains(t, r.List[1:], ro.List[0])

	// with two nodes, if the second is the root, the first should end up
	// in r.List[0]
	names = genLocalhostPeerNames(2, 0)
	ro = genRoster(tSuite, names)
	assert.Equal(t, len(ro.List), 2)
	ro.List[1] = network.NewServerIdentity(ro.List[1].Public, ro.List[1].Address)
	// The bug turned out to be about comparing, so let's give a root that is the same
	// server id, but a different pointer.
	r = ro.RandomSubset(network.NewServerIdentity(ro.List[1].Public, ro.List[1].Address), 1)
	assert.Equal(t, len(r.List), 2)
	assert.Equal(t, r.List[0], ro.List[1])
	assert.Equal(t, r.List[1], ro.List[0])
	// Check the "star" topology of these two guys
	// is root==0 -> (len(child)==1) && child[0]==1
	tr := r.GenerateStar()
	assert.Equal(t, tr.Root.ServerIdentity, r.List[0])
	assert.Equal(t, len(tr.Root.Children), 1)
	assert.Equal(t, tr.Root.Children[0].ServerIdentity, r.List[1])
}

// test the ID generation
func TestTreeId(t *testing.T) {
	names := genLocalhostPeerNames(3, 0)
	idsList := genRoster(tSuite, names)
	// Generate two example topology
	tree := idsList.GenerateBinaryTree()
	/*
			TODO: re-calculate the uuid
		root, _ := ExampleGenerateTreeFromRoster(idsList)
		tree := Tree{IdList: idsList, Root: root}
		var h bytes.Buffer
		h.Write(idsList.Id().Bytes())
		h.Write(root.Id().Bytes())
		genId := uuid.NewV5(uuid.NamespaceURL, h.String())
		if !uuid.Equal(genId, tree.Id()) {
				t.Fatal("Id generated is wrong")
			}
	*/
	if len(tree.ID.String()) != 36 {
		t.Fatal("Id generated is wrong")
	}
}

// Test if topology correctly handles the "virtual" connections in the topology
func TestTreeConnectedTo(t *testing.T) {
	names := genLocalhostPeerNames(3, 0)
	peerList := genRoster(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	// Generate the network
	if !tree.Root.IsConnectedTo(peerList.List[1]) {
		t.Fatal("Root should be connected to child (localhost:2001)")
	}
	if !tree.Root.IsConnectedTo(peerList.List[2]) {
		t.Fatal("Root should be connected to child (localhost:2002)")
	}
}

// Test initialisation of new peer-list
func TestRosterNew(t *testing.T) {
	adresses := genLocalhostPeerNames(2, 2000)
	pl := genRoster(tSuite, adresses)
	if len(pl.List) != 2 {
		t.Fatalf("Expected two peers in PeerList. Instead got %d", len(pl.List))
	}
	if pl.ID.IsNil() {
		t.Fatal("PeerList without ID is not allowed")
	}
	if len(pl.ID.String()) != 36 {
		t.Fatal("PeerList ID does not seem to be a UUID.")
	}
}

// Test initialisation of new peer-list from config-file
func TestInitPeerListFromConfigFile(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	idsList := genRoster(tSuite, names)
	// write it
	tmpDir, err := ioutil.TempDir("", "tree_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	WriteTomlConfig(idsList.Toml(tSuite), "identities.toml", tmpDir)
	// decode it
	var decoded RosterToml
	if err := ReadTomlConfig(&decoded, "identities.toml", tmpDir); err != nil {
		t.Fatal("Could not read from file the entityList")
	}
	decodedList := decoded.Roster(tSuite)
	if len(decodedList.List) != 3 {
		t.Fatalf("Expected two identities in Roster. Instead got %d", len(decodedList.List))
	}
	if decodedList.ID.IsNil() {
		t.Fatal("PeerList without ID is not allowed")
	}
	if len(decodedList.ID.String()) != 36 {
		t.Fatal("PeerList ID does not seem to be a UUID hash.")
	}
}

// Test initialisation of new random tree from a peer-list

// Test initialisation of new graph from config-file using a peer-list
// XXX again this test might be obsolete/does more harm then it is useful:
// It forces every field to be exported/made public
// and we want to get away from config files (or not?)

// Test initialisation of new graph when one peer is represented more than
// once

// Test access to tree:
// - parent
func TestTreeParent(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	peerList := genRoster(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	child := tree.Root.Children[0]
	if !child.Parent.ID.Equal(tree.Root.ID) {
		t.Fatal("Parent of child of root is not the root...")
	}
}

// - children
func TestTreeChildren(t *testing.T) {
	names := genLocalhostPeerNames(2, 2000)
	peerList := genRoster(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	child := tree.Root.Children[0]
	if !child.ServerIdentity.ID.Equal(peerList.List[1].ID) {
		t.Fatal("Parent of child of root is not the root...")
	}
}

// Test marshal/unmarshaling of trees
func TestUnMarshalTree(t *testing.T) {
	names := genLocalhostPeerNames(10, 2000)
	peerList := genRoster(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	treeBinary, err := tree.Marshal()

	if err != nil {
		t.Fatal("Error while marshaling:", err)
	}
	if len(treeBinary) == 0 {
		t.Fatal("Marshaled tree is empty")
	}

	tree2, err := NewTreeFromMarshal(tSuite, treeBinary, peerList)
	if err != nil {
		t.Fatal("Error while unmarshaling:", err)
	}
	if !tree.Equal(tree2) {
		log.Lvl3(tree, "\n", tree2)
		t.Fatal("Tree and Tree2 are not identical")
	}
}

func TestGetNode(t *testing.T) {
	tree, _ := genLocalTree(10, 2000)
	for _, tn := range tree.List() {
		node := tree.Search(tn.ID)
		if node == nil {
			t.Fatal("Didn't find treeNode with id", tn.ID)
		}
	}
}

func TestBinaryTree(t *testing.T) {
	tree, _ := genLocalTree(7, 2000)
	root := tree.Root
	if len(root.Children) != 2 {
		t.Fatal("Not two children from root")
	}
	if len(root.Children[0].Children) != 2 {
		t.Fatal("Not two children from first child")
	}
	if len(root.Children[1].Children) != 2 {
		t.Fatal("Not two children from second child")
	}
	if !tree.IsBinary(root) {
		t.Fatal("Tree should be binary")
	}
}

func TestTreeNodeServerIdentityIndex(t *testing.T) {
	names := genLocalhostPeerNames(13, 2000)
	peerList := genRoster(tSuite, names)
	tree := peerList.GenerateNaryTree(3)

	ln := tree.List()
	for _, node := range ln {
		idx := -1
		for i, e := range peerList.List {
			if e.Equal(node.ServerIdentity) {
				idx = i
				break
			}
		}

		if idx == -1 {
			t.Fatal("Could not find the entity in the node")
		}

		if node.RosterIndex != idx {
			t.Fatal("Index of entity do not correlate")
		}
	}
}

func TestNaryTree(t *testing.T) {
	names := genLocalhostPeerNames(13, 2000)
	peerList := genRoster(tSuite, names)
	tree := peerList.GenerateNaryTree(3)
	root := tree.Root
	if len(root.Children) != 3 {
		t.Fatal("Not three children from root")
	}
	if len(root.Children[0].Children) != 3 {
		t.Fatal("Not three children from first child")
	}
	if len(root.Children[1].Children) != 3 {
		t.Fatal("Not three children from second child")
	}
	if len(root.Children[2].Children) != 3 {
		t.Fatal("Not three children from third child")
	}
	if !tree.IsNary(root, 3) {
		t.Fatal("Tree should be 3-ary")
	}

	names = genLocalhostPeerNames(14, 0)
	peerList = genRoster(tSuite, names)
	tree = peerList.GenerateNaryTree(3)
	root = tree.Root
	if len(root.Children) != 3 {
		t.Fatal("Not three children from root")
	}
	if len(root.Children[0].Children) != 3 {
		t.Fatal("Not three children from first child")
	}
	if len(root.Children[1].Children) != 3 {
		t.Fatal("Not three children from second child")
	}
	if len(root.Children[2].Children) != 3 {
		t.Fatal("Not three children from third child")
	}
}

func TestBigNaryTree(t *testing.T) {
	names := genLocalDiffPeerNames(3, 2000)
	peerList := genRoster(tSuite, names)
	tree := peerList.GenerateBigNaryTree(3, 13)
	root := tree.Root
	log.Lvl2(tree.Dump())
	if !tree.IsNary(root, 3) {
		t.Fatal("Tree should be 3-ary")
	}
	for _, child := range root.Children {
		if child.ServerIdentity.ID.Equal(root.ServerIdentity.ID) {
			t.Fatal("Child should not have same identity as parent")
		}
		for _, c := range child.Children {
			if c.ServerIdentity.ID.Equal(child.ServerIdentity.ID) {
				t.Fatal("Child should not have same identity as parent")
			}
		}
	}
}

func TestTreeIsColored(t *testing.T) {
	names := genLocalPeerName(2, 2)
	peerList := genRoster(tSuite, names)
	tree := peerList.GenerateBigNaryTree(3, 13)
	root := tree.Root
	rootHost := root.ServerIdentity.Address.Host()
	for _, child := range root.Children {
		childHost := child.ServerIdentity.Address.NetworkAddress()
		if rootHost == childHost {
			t.Fatal("Child", childHost, "is the same as root", rootHost)
		}
	}
}

func TestBinaryTrees(t *testing.T) {
	tree, _ := genLocalTree(1, 2000)
	if !tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 1 node should be binary")
	}
	tree, _ = genLocalTree(2, 0)
	if tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 2 nodes should NOT be binary")
	}
	tree, _ = genLocalTree(3, 0)
	if !tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 3 nodes should be binary")
	}
	tree, _ = genLocalTree(4, 0)
	if tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 4 nodes should NOT be binary")
	}
}

func TestRosterIsUsed(t *testing.T) {
	port := 2000
	for hostExp := uint(2); hostExp < 8; hostExp++ {
		hosts := (1 << hostExp) - 1
		log.Lvl2("Trying tree with", hosts, "hosts")
		names := make([]network.Address, hosts)
		for i := 0; i < hosts; i++ {
			add := "localhost" + strconv.Itoa(i/2) + ":" +
				strconv.Itoa(port+i)
			names[i] = network.NewAddress(network.Local, add)

		}
		peerList := genRoster(tSuite, names)
		tree := peerList.GenerateBigNaryTree(2, hosts)
		if !tree.UsesList() {
			t.Fatal("Didn't find all ServerIdentities in tree", tree.Dump())
		}
	}
}

// Test whether the computation of the subtree aggregate public key is correct .
func TestTreeComputeSubtreeAggregate(t *testing.T) {
	names := genLocalDiffPeerNames(7, 2000)
	entities := genRoster(tSuite, names)

	// create tree
	tree := entities.GenerateBinaryTree()

	// manual check for 2nd level of tree (left part)
	lchild := tree.Root.Children[0]
	n2, n4, n5 := lchild.ServerIdentity, lchild.Children[0].ServerIdentity, lchild.Children[1].ServerIdentity
	aggLeft := tSuite.Point().Add(n2.Public, n4.Public)
	aggLeft = aggLeft.Add(aggLeft, n5.Public)
	if !tree.Root.Children[0].PublicAggregateSubTree.Equal(aggLeft) {
		t.Fatal("Aggregate is not correct for the left part")
	}

	// right part
	rchild := tree.Root.Children[1]
	n3, n4, n5 := rchild.ServerIdentity, rchild.Children[0].ServerIdentity, rchild.Children[1].ServerIdentity
	aggRight := tSuite.Point().Add(n3.Public, n4.Public)
	aggRight = aggRight.Add(aggRight, n5.Public)
	if !tree.Root.Children[1].PublicAggregateSubTree.Equal(aggRight) {
		t.Fatal("Aggregate is not correct for the right part")
	}

	// root part
	agg := tSuite.Point().Add(aggRight, aggLeft)
	agg = agg.Add(agg, tree.Root.ServerIdentity.Public)
	if !tree.Root.PublicAggregateSubTree.Equal(agg) {
		t.Fatal("Aggregate not correct for root")
	}

}

func TestTree_BinaryMarshaler(t *testing.T) {
	tree, _ := genLocalTree(5, 2000)
	b, err := tree.BinaryMarshaler()
	log.ErrFatal(err)
	tree2 := &Tree{}
	log.ErrFatal(tree2.BinaryUnmarshaler(tSuite, b))
	if !tree.Equal(tree2) {
		t.Fatal("Unmarshalled tree is not equal")
	}
	if tree.Root == tree2.Root {
		t.Fatal("Address should not be equal")
	}
	log.Lvl1(tree.Dump())
	log.Lvl1(tree2.Dump())
}

func TestTreeNode_SubtreeCount(t *testing.T) {
	tree, _ := genLocalTree(15, 2000)
	if tree.Root.SubtreeCount() != 14 {
		t.Fatal("Not enough nodes in subtree-count")
	}
	if tree.Root.Children[0].SubtreeCount() != 6 {
		t.Fatal("Not enough nodes in partial subtree")
	}
	if tree.Root.Children[0].Children[0].SubtreeCount() != 2 {
		t.Fatal("Not enough nodes in partial subtree")
	}
	if tree.Root.Children[0].Children[0].Children[0].SubtreeCount() != 0 {
		t.Fatal("Not enough nodes in partial subtree")
	}
}

// Deprecated: the ID should be gotten using GetID
func TestRoster_ID(t *testing.T) {
	names := genLocalhostPeerNames(10, 2000)
	ro := genRoster(tSuite, names)
	ro2 := NewRoster(ro.List)

	assert.True(t, ro.ID.Equal(ro2.ID))

	// check missing service identities
	tt := []*network.ServerIdentity{}
	for _, id := range ro.List {
		tt = append(tt, network.NewServerIdentity(id.Public, id.Address))
	}

	ro3 := NewRoster(tt)
	assert.False(t, ro3.ID.Equal(ro.ID))
}

func TestRoster_GetID(t *testing.T) {
	names := genLocalhostPeerNames(10, 2000)
	ro := genRoster(tSuite, names)
	ro2 := NewRoster(ro.List)

	roID, err := ro.GetID()
	require.NoError(t, err)
	ro2ID, err := ro2.GetID()
	require.NoError(t, err)
	require.True(t, roID.Equal(ro2ID))
	ok, _ := ro.Equal(ro2)
	require.True(t, ok)

	// check unordered service identities
	ro.List[0].ServiceIdentities[0], ro.List[0].ServiceIdentities[1] = ro.List[0].ServiceIdentities[1], ro.List[0].ServiceIdentities[0]
	ro3 := NewRoster(ro.List)
	ro3ID, err := ro3.GetID()
	require.NoError(t, err)
	require.True(t, roID.Equal(ro3ID))
	ok, _ = ro.Equal(ro3)
	require.True(t, ok)

	// check missing service identities
	tt := []*network.ServerIdentity{}
	for _, id := range ro.List {
		tt = append(tt, network.NewServerIdentity(id.Public, id.Address))
	}

	ro4 := NewRoster(tt)
	ro4ID, err := ro4.GetID()
	require.NoError(t, err)
	require.False(t, ro4ID.Equal(roID))
	ok, _ = ro.Equal(ro4)
	require.False(t, ok)
}

func TestRoster_GenerateNaryTree(t *testing.T) {
	names := genLocalhostPeerNames(10, 2000)
	peerList := genRoster(tSuite, names)
	peerList.GenerateNaryTree(4)
	for i := 0; i <= 9; i++ {
		if !strings.Contains(peerList.List[i].Address.String(),
			strconv.Itoa(2000+i)) {
			t.Fatal("Missing port:", 2000+i, peerList.List)
		}
	}
}

func TestRoster_GenerateNaryTreeWithRoot_NewRoster(t *testing.T) {
	names := genLocalhostPeerNames(10, 2000)
	peerList := genRoster(tSuite, names)
	tree := NewRoster(peerList.List[1:10]).GenerateNaryTreeWithRoot(2, peerList.List[0])
	assert.Nil(t, tree)
	for _, e := range peerList.List[1:len(peerList.List)] {
		tree := peerList.NewRosterWithRoot(e).GenerateNaryTree(4)
		for i := 0; i <= 9; i++ {
			if !strings.Contains(peerList.List[i].Address.String(),
				strconv.Itoa(2000+i)) {
				t.Fatal("Missing port:", 2000+i, peerList.List)
			}
		}
		if !tree.Root.ServerIdentity.ID.Equal(e.ID) {
			t.Fatal("ServerIdentity", e, "is not root", tree.Dump())
		}
		if len(tree.List()) != 10 {
			t.Fatal("Missing nodes")
		}
		if !tree.UsesList() {
			t.Fatal("Not all elements are in the tree")
		}
		if tree.Roster.ID == peerList.ID {
			t.Fatal("Generated tree should not be associated the receiver roster")
		}
	}
}

func TestRoster_GenerateNaryTreeWithRoot(t *testing.T) {
	names := genLocalhostPeerNames(10, 2000)
	peerList := genRoster(tSuite, names)
	tree := NewRoster(peerList.List[1:10]).GenerateNaryTreeWithRoot(2, peerList.List[0])
	assert.Nil(t, tree)
	for _, e := range peerList.List {
		tree := peerList.GenerateNaryTreeWithRoot(4, e)
		for i := 0; i <= 9; i++ {
			if !strings.Contains(peerList.List[i].Address.String(),
				strconv.Itoa(2000+i)) {
				t.Fatal("Missing port:", 2000+i, peerList.List)
			}
		}
		if !tree.Root.ServerIdentity.ID.Equal(e.ID) {
			t.Fatal("ServerIdentity", e, "is not root", tree.Dump())
		}
		if len(tree.List()) != 10 {
			t.Fatal("Missing nodes")
		}
		if !tree.UsesList() {
			t.Fatal("Not all elements are in the tree")
		}
		if tree.Roster.ID != peerList.ID {
			t.Fatal("Generated tree should be associated with the receiver roster")
		}
	}
}

func TestRoster_Publics(t *testing.T) {
	_, roster := genLocalTree(5, 2000)
	agg := roster.Publics()
	spk := roster.ServicePublics("")

	for i, si := range roster.List {
		if !agg[i].Equal(si.Public) {
			t.Fatal("Aggregate of 5 keys is not correct")
		}

		require.True(t, spk[i].Equal(si.Public))
	}

	_, roster = genLocalTree(2, 0)
	agg = roster.Publics()
	agg2 := roster.List[0].Public.Add(roster.List[0].Public,
		roster.List[1].Public)
	if !agg[0].Equal(agg2) {
		t.Fatal("Aggregate of 2 keys is not correct")
	}
}

func TestRoster_IsRotation(t *testing.T) {
	n := 5
	_, tmpRoster := genLocalTree(n, 2000)
	roster := NewRoster(tmpRoster.List)

	// a roster that is missing an element is not a valid rotation
	rosterShort := NewRoster(roster.List[1:])
	// a roster with the final two elements swapped is not a valid rotation
	rosterSwapped := NewRoster(append(roster.List[0:n-2], roster.List[n-1], roster.List[n-2]))
	// the following are valid rotations
	rosterRotated0 := NewRoster(append(roster.List[1:], roster.List[0]))
	rosterRotated1 := NewRoster(append(rosterRotated0.List[1:], rosterRotated0.List[0]))

	assert.False(t, roster.IsRotation(nil))
	assert.False(t, roster.IsRotation(rosterShort))
	assert.False(t, roster.IsRotation(rosterSwapped))
	assert.True(t, roster.IsRotation(rosterRotated0))
	assert.True(t, roster.IsRotation(rosterRotated1))
}

func TestRoster_Contains(t *testing.T) {
	_, roster := genLocalTree(10, 2000)

	pubs := roster.Publics()
	require.True(t, roster.Contains(pubs))

	for i := 0; i < 10; i++ {
		rand.Shuffle(len(pubs), func(i, j int) {
			pubs[i], pubs[j] = pubs[j], pubs[i]
			require.True(t, roster.Contains(pubs))
		})
	}

	require.False(t, roster.Contains(pubs[1:]))
}

// Checks that you can concatenate two rosters together
// without duplicates
func TestRoster_Concat(t *testing.T) {
	_, roster := genLocalTree(10, 2000)

	r1 := NewRoster(roster.List[:7])
	r2 := NewRoster(roster.List[2:])

	r := r1.Concat(r2.List...)
	require.Equal(t, 10, len(r.List))
	require.True(t, r.Contains(roster.Publics()))

	r = r1.Concat()
	require.Equal(t, len(r1.List), len(r.List))
}

func TestTreeNode_AggregatePublic(t *testing.T) {
	tree, el := genLocalTree(7, 2000)
	agg := el.Aggregate
	root := tree.Root
	aggRoot := root.AggregatePublic(tSuite)
	assert.True(t, aggRoot.Equal(agg))

	rootPub := tree.Root.ServerIdentity.Public
	aggChild1 := tree.Root.Children[0].AggregatePublic(tSuite)
	aggChild2 := tree.Root.Children[1].AggregatePublic(tSuite)

	assert.True(t, aggChild1.Add(aggChild1, aggChild2).
		Add(aggChild1, rootPub).Equal(aggRoot))

	for i := 0; i < 4; i++ {
		leaf := tree.Root.Children[i%2].Children[i/2]
		assert.True(t, leaf.AggregatePublic(tSuite).Equal(leaf.ServerIdentity.Public))
	}
}

func TestRoster_ServiceAggregate(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	ro := genRoster(tSuite, names)
	_, err := ro.ServiceAggregate("unknown")
	require.Error(t, err)
	_, err = ro.ServiceAggregate("ServiceTest")
	require.NoError(t, err)
}

// BenchmarkTreeMarshal will be the benchmark for the conversion between TreeMarshall and Tree
func BenchmarkTreeMarshal(b *testing.B) {
	tree, _ := genLocalTree(1000, 0)
	t, _ := tree.BinaryMarshaler()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.BinaryUnmarshaler(tSuite, t)
	}
}

// BenchmarkMakeTree will time the amount of time it will take to make the tree
func BenchmarkMakeTree(b *testing.B) {
	tree, _ := genLocalTree(1000, 0)
	el := tree.Roster
	T := tree.MakeTreeMarshal()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		T.MakeTree(el)
	}
}

// BenchmarkUnmarshalRegisteredType will time the amout it takes to perform the UnmarshalRegisteredType
func BenchmarkUnmarshalRegisteredType(b *testing.B) {
	tree, _ := genLocalTree(1000, 0)
	buf, _ := tree.BinaryMarshaler()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = network.Unmarshal(buf, tSuite)
	}
}

// BenchmarkBinaryMarshaller will time the binary marshaler in order to compare MarshalerRegisteredType(used within BinaryMarshaler) to the UnmarshalRegisteredType
func BenchmarkBinaryMarshaler(b *testing.B) {
	tree, _ := genLocalTree(1000, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.BinaryMarshaler()
	}
}

// genLocalhostPeerNames will generate n localhost names with port indices starting from p
func genLocalhostPeerNames(n, p int) []network.Address {
	names := make([]network.Address, n)
	for i := range names {
		names[i] = network.NewAddress(network.Local, prefix+strconv.Itoa(p+i))
	}
	return names
}

// genLocalDiffPeerNames will generate n local0..n-1 names with port indices starting from p
func genLocalDiffPeerNames(n, p int) []network.Address {
	names := make([]network.Address, n)
	for i := range names {
		names[i] = network.NewTCPAddress("127.0.0." + strconv.Itoa(i) + ":2000")
	}
	return names
}

// genLocalPeerName takes
// nbrLocal: number of different local host address should it generate
// nbrPort: for each different local host address, how many addresses with
// different port should it generate
// ex: genLocalPeerName(2,2) => local1:2000,local1:2001, local2:2000,local2:2001
func genLocalPeerName(nbrLocal, nbrPort int) []network.Address {
	names := make([]network.Address, nbrLocal)
	for i := range names {
		names[i] = network.NewAddress(network.Local, "127.0.0."+strconv.Itoa(i)+":2000")
	}
	return names

}

// genRoster generates a Roster out of names
func genRoster(suite suites.Suite, names []network.Address) *Roster {
	var ids []*network.ServerIdentity
	for _, n := range names {
		kp := key.NewKeyPair(suite)
		srvid := network.NewServerIdentity(kp.Public, n)
		srvid.ServiceIdentities = []network.ServiceIdentity{
			genServiceIdentity("ServiceTest", suite),
			genServiceIdentity("AnotherServiceTest", suite),
		}

		ids = append(ids, srvid)
	}
	return NewRoster(ids)
}

func genServiceIdentity(name string, suite suites.Suite) network.ServiceIdentity {
	kp := key.NewKeyPair(suite)

	return network.NewServiceIdentityFromPair(name, suite, kp)
}

func genLocalTree(count, port int) (*Tree, *Roster) {
	names := genLocalhostPeerNames(count, port)
	peerList := genRoster(tSuite, names)
	tree := peerList.GenerateBinaryTree()
	return tree, peerList
}
