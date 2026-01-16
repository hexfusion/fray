package merkle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTree(t *testing.T) {
	require := require.New(t)

	tree := New(10*1024*1024, 1024*1024)

	require.Equal(10, tree.NumChunks)
	require.False(tree.Complete())
	require.Equal(float64(0), tree.Progress())
}

func TestTreeSizes(t *testing.T) {
	tests := []struct {
		name       string
		blobSize   int64
		chunkSize  int
		wantChunks int
		complete   bool
		progress   float64
	}{
		{"empty blob", 0, 1024 * 1024, 0, true, 1.0},
		{"tiny blob", 100, 1024 * 1024, 1, false, 0},
		{"exact boundary", 4 * 1024 * 1024, 1024 * 1024, 4, false, 0},
		{"partial last chunk", 2*1024*1024 + 512*1024, 1024 * 1024, 3, false, 0},
		{"large blob", 1024 * 1024 * 1024, 1024 * 1024, 1024, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			tree := New(tt.blobSize, tt.chunkSize)

			require.Equal(tt.wantChunks, tree.NumChunks)
			require.Equal(tt.complete, tree.Complete())
			require.Equal(tt.progress, tree.Progress())
		})
	}
}

func TestChunkOffsetAndLength(t *testing.T) {
	tree := New(2*1024*1024+512*1024, 1024*1024)

	tests := []struct {
		chunk      int
		wantOffset int64
		wantLength int
	}{
		{0, 0, 1024 * 1024},
		{1, 1024 * 1024, 1024 * 1024},
		{2, 2 * 1024 * 1024, 512 * 1024},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			require := require.New(t)
			require.Equal(tt.wantOffset, tree.ChunkOffset(tt.chunk))
			require.Equal(tt.wantLength, tree.ChunkLength(tt.chunk))
		})
	}
}

func TestSetChunk(t *testing.T) {
	require := require.New(t)

	tree := New(4*1024*1024, 1024*1024)

	require.NoError(tree.SetChunk(0, []byte("test chunk data")))
	require.True(tree.HasChunk(0))
	require.False(tree.HasChunk(1))
	require.Equal(0.25, tree.Progress())
}

func TestSetChunkBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		index   int
		wantErr bool
	}{
		{"negative index", -1, true},
		{"beyond range", 4, true},
		{"first valid", 0, false},
		{"last valid", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			tree := New(4*1024*1024, 1024*1024)
			err := tree.SetChunk(tt.index, []byte("data"))

			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestSetChunkDuplicate(t *testing.T) {
	require := require.New(t)

	tree := New(2*1024*1024, 1024*1024)

	require.NoError(tree.SetChunk(0, []byte("data")))
	require.NoError(tree.SetChunk(0, []byte("data")))
	require.Equal(1, tree.PresentCount)

	require.NoError(tree.SetChunk(0, []byte("different data")))
	require.Equal(1, tree.PresentCount)
}

func TestMissingRanges(t *testing.T) {
	tests := []struct {
		name       string
		numChunks  int
		setChunks  []int
		wantRanges [][2]int
	}{
		{
			name:       "all missing",
			numChunks:  5,
			setChunks:  nil,
			wantRanges: [][2]int{{0, 5}},
		},
		{
			name:       "none missing",
			numChunks:  3,
			setChunks:  []int{0, 1, 2},
			wantRanges: nil,
		},
		{
			name:       "gaps in middle",
			numChunks:  10,
			setChunks:  []int{0, 1, 2, 5, 6},
			wantRanges: [][2]int{{3, 5}, {7, 10}},
		},
		{
			name:       "first chunk missing",
			numChunks:  4,
			setChunks:  []int{1, 2, 3},
			wantRanges: [][2]int{{0, 1}},
		},
		{
			name:       "last chunk missing",
			numChunks:  4,
			setChunks:  []int{0, 1, 2},
			wantRanges: [][2]int{{3, 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			tree := New(int64(tt.numChunks)*1024*1024, 1024*1024)

			for _, i := range tt.setChunks {
				require.NoError(tree.SetChunk(i, []byte("data")))
			}

			ranges := tree.MissingRanges()

			if tt.wantRanges == nil {
				require.Empty(ranges)
			} else {
				require.Equal(tt.wantRanges, ranges)
			}
		})
	}
}

func TestRoot(t *testing.T) {
	require := require.New(t)

	tree := New(2*1024*1024, 1024*1024)

	root1 := tree.Root()
	require.False(root1.IsEmpty())

	require.NoError(tree.SetChunk(0, []byte("chunk 0 data")))
	root2 := tree.Root()
	require.NotEqual(root1, root2)

	require.NoError(tree.SetChunk(0, []byte("chunk 0 data")))
	root3 := tree.Root()
	require.Equal(root2, root3)
}

func TestHashDeterminism(t *testing.T) {
	require := require.New(t)

	tree1 := New(2*1024*1024, 1024*1024)
	tree2 := New(2*1024*1024, 1024*1024)

	require.NoError(tree1.SetChunk(0, []byte("data A")))
	require.NoError(tree2.SetChunk(0, []byte("data A")))
	require.Equal(tree1.ChunkHash(0), tree2.ChunkHash(0))

	require.NoError(tree1.SetChunk(1, []byte("data B")))
	require.NotEqual(tree1.ChunkHash(0), tree1.ChunkHash(1))
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name        string
		tree1Chunks []int
		tree2Chunks []int
		tree2Size   int64
		wantSend    []int
		wantReceive []int
	}{
		{
			name:        "complementary sets",
			tree1Chunks: []int{0, 1},
			tree2Chunks: []int{2, 3},
			tree2Size:   4 * 1024 * 1024,
			wantSend:    []int{0, 1},
			wantReceive: []int{2, 3},
		},
		{
			name:        "identical sets",
			tree1Chunks: []int{0, 1},
			tree2Chunks: []int{0, 1},
			tree2Size:   4 * 1024 * 1024,
			wantSend:    nil,
			wantReceive: nil,
		},
		{
			name:        "size mismatch",
			tree1Chunks: []int{0, 1},
			tree2Chunks: []int{0, 1},
			tree2Size:   8 * 1024 * 1024,
			wantSend:    nil,
			wantReceive: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			tree1 := New(4*1024*1024, 1024*1024)
			tree2 := New(tt.tree2Size, 1024*1024)

			for _, i := range tt.tree1Chunks {
				require.NoError(tree1.SetChunk(i, []byte("data")))
			}
			for _, i := range tt.tree2Chunks {
				require.NoError(tree2.SetChunk(i, []byte("data")))
			}

			toSend, toReceive := tree1.Diff(tree2)

			if tt.wantSend == nil {
				require.Nil(toSend)
			} else {
				require.Equal(tt.wantSend, toSend)
			}
			if tt.wantReceive == nil {
				require.Nil(toReceive)
			} else {
				require.Equal(tt.wantReceive, toReceive)
			}
		})
	}
}

func TestSerializeDeserialize(t *testing.T) {
	require := require.New(t)

	tree := New(4*1024*1024, 1024*1024)
	require.NoError(tree.SetChunk(0, []byte("data 0")))
	require.NoError(tree.SetChunk(2, []byte("data 2")))

	originalRoot := tree.Root()

	state := tree.Serialize()
	restored, err := Deserialize(state)
	require.NoError(err)

	require.Equal(tree.NumChunks, restored.NumChunks)
	require.Equal(tree.PresentCount, restored.PresentCount)
	require.Equal(originalRoot, restored.Root())
	require.True(restored.HasChunk(0))
	require.False(restored.HasChunk(1))
	require.True(restored.HasChunk(2))
	require.False(restored.HasChunk(3))
}

func TestFileRoundTrip(t *testing.T) {
	require := require.New(t)

	tree := New(4*1024*1024, 1024*1024)
	require.NoError(tree.SetChunk(0, []byte("chunk 0")))
	require.NoError(tree.SetChunk(2, []byte("chunk 2")))

	tmpFile := t.TempDir() + "/tree.json"
	require.NoError(tree.SaveToFile(tmpFile))

	loaded, err := LoadFromFile(tmpFile)
	require.NoError(err)

	require.Equal(tree.Root(), loaded.Root())
	require.Equal(tree.PresentCount, loaded.PresentCount)
}

func TestProgress(t *testing.T) {
	tests := []struct {
		name         string
		numChunks    int
		setChunks    int
		wantProgress float64
	}{
		{"none set", 4, 0, 0},
		{"quarter set", 4, 1, 0.25},
		{"half set", 4, 2, 0.5},
		{"all set", 4, 4, 1.0},
		{"large blob half", 1024, 512, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			tree := New(int64(tt.numChunks)*1024*1024, 1024*1024)

			for i := 0; i < tt.setChunks; i++ {
				require.NoError(tree.SetChunk(i, []byte("data")))
			}

			require.Equal(tt.wantProgress, tree.Progress())
		})
	}
}
