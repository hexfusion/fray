package merkle

import (
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/cespare/xxhash/v2"
)

// Hash is an xxHash64 hash. final blob verification uses SHA-256 (OCI requirement),
// but merkle tree uses xxHash64 for speed on edge devices.
type Hash uint64

// EmptyHash represents a missing/empty chunk.
const EmptyHash Hash = 0

func (h Hash) String() string {
	return fmt.Sprintf("%016x", uint64(h))
}

func (h Hash) IsEmpty() bool {
	return h == EmptyHash
}

// HashFromHex parses a hex string into a hash.
func HashFromHex(s string) (Hash, error) {
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, err
	}
	return Hash(v), nil
}

// HashData computes the xxHash64 of data.
func HashData(data []byte) Hash {
	return Hash(xxhash.Sum64(data))
}

// Tree is a merkle tree for tracking chunk state.
type Tree struct {
	// total size of the blob being chunked
	TotalSize int64
	// size of each chunk (except possibly the last)
	ChunkSize int
	// number of chunks (leaves)
	NumChunks int
	// leaf hashes (chunk hashes or EmptyHash if missing)
	Leaves []Hash
	// number of chunks that are present
	PresentCount int
}

// New creates a new merkle tree for a blob of the given size.
func New(totalSize int64, chunkSize int) *Tree {
	numChunks := int((totalSize + int64(chunkSize) - 1) / int64(chunkSize))
	leafCount := nextPowerOf2(numChunks)

	return &Tree{
		TotalSize: totalSize,
		ChunkSize: chunkSize,
		NumChunks: numChunks,
		Leaves:    make([]Hash, leafCount),
	}
}

// SetChunk marks a chunk as present with its hash.
func (t *Tree) SetChunk(index int, data []byte) error {
	if index < 0 || index >= t.NumChunks {
		return fmt.Errorf("chunk index %d out of range [0, %d)", index, t.NumChunks)
	}

	h := HashData(data)
	wasEmpty := t.Leaves[index].IsEmpty()
	t.Leaves[index] = h
	if wasEmpty {
		t.PresentCount++
	}

	return nil
}

// ClearChunk marks a chunk as missing (for re-download after corruption).
func (t *Tree) ClearChunk(index int) {
	if index < 0 || index >= t.NumChunks {
		return
	}

	if !t.Leaves[index].IsEmpty() {
		t.Leaves[index] = EmptyHash
		t.PresentCount--
	}
}

// SetChunkHash marks a chunk as present with a precomputed hash.
func (t *Tree) SetChunkHash(index int, h Hash) error {
	if index < 0 || index >= t.NumChunks {
		return fmt.Errorf("chunk index %d out of range [0, %d)", index, t.NumChunks)
	}

	wasEmpty := t.Leaves[index].IsEmpty()
	t.Leaves[index] = h

	if wasEmpty && !h.IsEmpty() {
		t.PresentCount++
	}

	return nil
}

// HasChunk returns true if the chunk is present.
func (t *Tree) HasChunk(index int) bool {
	if index < 0 || index >= t.NumChunks {
		return false
	}
	return !t.Leaves[index].IsEmpty()
}

// ChunkHash returns the hash of a chunk, or empty hash if missing.
func (t *Tree) ChunkHash(index int) Hash {
	if index < 0 || index >= t.NumChunks {
		return EmptyHash
	}
	return t.Leaves[index]
}

// Root computes the merkle root hash.
func (t *Tree) Root() Hash {
	if len(t.Leaves) == 0 {
		return EmptyHash
	}

	level := make([]Hash, len(t.Leaves))
	copy(level, t.Leaves)

	for len(level) > 1 {
		nextLevel := make([]Hash, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			nextLevel[i/2] = hashPair(level[i], level[i+1])
		}
		level = nextLevel
	}

	return level[0]
}

// MissingChunks returns the indices of all missing chunks.
func (t *Tree) MissingChunks() []int {
	missing := make([]int, 0, t.NumChunks-t.PresentCount)
	for i := 0; i < t.NumChunks; i++ {
		if t.Leaves[i].IsEmpty() {
			missing = append(missing, i)
		}
	}
	return missing
}

// MissingRanges returns contiguous ranges of missing chunks as [start, end).
func (t *Tree) MissingRanges() [][2]int {
	var ranges [][2]int
	inRange := false
	start := 0

	for i := 0; i < t.NumChunks; i++ {
		missing := t.Leaves[i].IsEmpty()
		if missing && !inRange {
			start = i
			inRange = true
		} else if !missing && inRange {
			ranges = append(ranges, [2]int{start, i})
			inRange = false
		}
	}

	if inRange {
		ranges = append(ranges, [2]int{start, t.NumChunks})
	}

	return ranges
}

// Complete returns true if all chunks are present.
func (t *Tree) Complete() bool {
	return t.PresentCount >= t.NumChunks
}

// Progress returns the fraction of chunks that are present.
func (t *Tree) Progress() float64 {
	if t.NumChunks == 0 {
		return 1.0
	}
	return float64(t.PresentCount) / float64(t.NumChunks)
}

// Diff compares this tree with another and returns chunks that differ.
func (t *Tree) Diff(other *Tree) (toSend, toReceive []int) {
	if t.NumChunks != other.NumChunks {
		return nil, nil
	}

	for i := 0; i < t.NumChunks; i++ {
		thisHas := !t.Leaves[i].IsEmpty()
		otherHas := !other.Leaves[i].IsEmpty()

		if thisHas && !otherHas {
			toSend = append(toSend, i)
		} else if !thisHas && otherHas {
			toReceive = append(toReceive, i)
		}
	}

	return toSend, toReceive
}

// ChunkOffset returns the byte offset of a chunk in the blob.
func (t *Tree) ChunkOffset(index int) int64 {
	return int64(index) * int64(t.ChunkSize)
}

// ChunkLength returns the length of a chunk.
func (t *Tree) ChunkLength(index int) int {
	if index < 0 || index >= t.NumChunks {
		return 0
	}

	offset := t.ChunkOffset(index)
	remaining := t.TotalSize - offset

	if remaining > int64(t.ChunkSize) {
		return t.ChunkSize
	}
	return int(remaining)
}

func hashPair(left, right Hash) Hash {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], uint64(left))
	binary.LittleEndian.PutUint64(buf[8:], uint64(right))
	return Hash(xxhash.Sum64(buf[:]))
}

func nextPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p *= 2
	}
	return p
}
