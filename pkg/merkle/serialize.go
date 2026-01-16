package merkle

import (
	"encoding/json"
	"os"
)

// State is the serializable form of a merkle tree.
type State struct {
	TotalSize    int64  `json:"total_size"`
	ChunkSize    int    `json:"chunk_size"`
	NumChunks    int    `json:"num_chunks"`
	PresentCount int    `json:"present_count"`
	Root         string `json:"root"`
	// hex-encoded hashes, empty string for missing
	Leaves []string `json:"leaves"`
}

// Serialize converts the tree to a JSON-serializable state.
func (t *Tree) Serialize() *State {
	leaves := make([]string, t.NumChunks)
	for i := 0; i < t.NumChunks; i++ {
		if !t.Leaves[i].IsEmpty() {
			leaves[i] = t.Leaves[i].String()
		}
	}

	return &State{
		TotalSize:    t.TotalSize,
		ChunkSize:    t.ChunkSize,
		NumChunks:    t.NumChunks,
		PresentCount: t.PresentCount,
		Root:         t.Root().String(),
		Leaves:       leaves,
	}
}

// Deserialize creates a tree from serialized state.
func Deserialize(s *State) (*Tree, error) {
	t := New(s.TotalSize, s.ChunkSize)
	t.PresentCount = 0

	for i, hexHash := range s.Leaves {
		if hexHash == "" {
			continue
		}

		h, err := HashFromHex(hexHash)
		if err != nil {
			return nil, err
		}

		t.Leaves[i] = h
		t.PresentCount++
	}

	return t, nil
}

// SaveToFile saves the tree state to a JSON file.
func (t *Tree) SaveToFile(path string) error {
	state := t.Serialize()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadFromFile loads a tree from a JSON state file.
func LoadFromFile(path string) (*Tree, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return Deserialize(&state)
}
