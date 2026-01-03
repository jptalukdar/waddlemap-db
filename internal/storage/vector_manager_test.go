package storage

import (
	"os"
	"testing"

	"waddlemap/internal/types"
)

func TestVectorManager_BlockOperations(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "vm_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &types.DBSchemaConfig{
		DataPath: tmpDir,
		SyncMode: "normal",
	}

	vm, err := NewVectorManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create VM: %v", err)
	}
	defer vm.Close()

	colName := "test_col"
	err = vm.CreateCollection(colName, 4, types.MetricL2)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// 1. Append Block
	key1 := "doc1"
	block1 := &types.BlockData{
		Primary:  "Hello World",
		Vector:   []float32{0.1, 0.2, 0.3, 0.4},
		Keywords: []string{"hello", "world"},
	}

	idx1, err := vm.AppendBlock(colName, key1, block1)
	if err != nil {
		t.Fatalf("AppendBlock failed: %v", err)
	}
	if idx1 != 0 {
		t.Errorf("Expected index 0, got %d", idx1)
	}

	// 2. Get Block
	retrievedBlock, err := vm.GetBlock(colName, key1, 0)
	if err != nil {
		t.Fatalf("GetBlock failed: %v", err)
	}
	if retrievedBlock.Primary != block1.Primary {
		t.Errorf("Primary mismatch: got %s, want %s", retrievedBlock.Primary, block1.Primary)
	}
	if len(retrievedBlock.Vector) != 4 {
		t.Errorf("Vector length mismatch")
	}

	// 3. Append Second Block
	block2 := &types.BlockData{
		Primary:  "Second Block",
		Vector:   []float32{0.5, 0.6, 0.7, 0.8},
		Keywords: []string{"second"},
	}
	idx2, err := vm.AppendBlock(colName, key1, block2)
	if err != nil {
		t.Fatalf("AppendBlock 2 failed: %v", err)
	}
	if idx2 != 1 {
		t.Errorf("Expected index 1, got %d", idx2)
	}

	// 4. Search
	results, err := vm.Search(colName, []float32{0.1, 0.2, 0.3, 0.4}, 1, "", nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	} else {
		res := results[0]
		if res.Key != key1 || res.Index != 0 {
			t.Errorf("Search result mismatch: Key %s Index %d", res.Key, res.Index)
		}
		if res.Block.Primary != block1.Primary {
			t.Errorf("Search result block missing")
		}
	}

	// 5. Delete Key
	err = vm.DeleteKey(colName, key1)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Verify Deletion
	exists, err := vm.ContainsKey(colName, key1)
	if err != nil {
		t.Fatalf("ContainsKey failed: %v", err)
	}
	if exists {
		t.Error("Key still exists after deletion")
	}

	// Verify Search returns nothing
	results, err = vm.Search(colName, []float32{0.1, 0.2, 0.3, 0.4}, 1, "", nil)
	if err != nil {
		t.Fatalf("Search after delete failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results after delete, got %d", len(results))
	}
}
