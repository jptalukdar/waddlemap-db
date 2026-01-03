package storage

import (
	"fmt"
	"log"
)

// RepairManager handles consistency checks and repairs for collections.
type RepairManager struct {
	cm *CollectionManager
}

// NewRepairManager creates a new repair manager.
func NewRepairManager(cm *CollectionManager) *RepairManager {
	return &RepairManager{cm: cm}
}

// RepairReport contains the results of a consistency check.
type RepairReport struct {
	Collection     string
	TotalVectors   int
	OrphanVectors  int // Vectors in HNSW but not in DocMap
	MissingVectors int // Entries in DocMap but not in HNSW
	OrphanIDs      []uint64
	MissingIDs     []uint64
	Repaired       bool
}

// CheckConsistency verifies that HNSW index and DocMap are in sync.
func (rm *RepairManager) CheckConsistency(collectionName string) (*RepairReport, error) {
	coll, err := rm.cm.GetCollection(collectionName)
	if err != nil {
		return nil, err
	}

	coll.mu.RLock()
	defer coll.mu.RUnlock()

	report := &RepairReport{
		Collection: collectionName,
	}

	// Get all vector IDs from DocMap
	// Accessing unexported field 'mapping' via package-level access
	coll.DocMap.mu.RLock()
	docMapIDs := make(map[uint64]bool)
	for vectorID := range coll.DocMap.mapping {
		docMapIDs[vectorID] = true
		report.TotalVectors++
	}
	coll.DocMap.mu.RUnlock()

	// Check HNSW for orphans (vectors not in DocMap)

	// Scanning strategy: Check defined range or iterate HNSW nodes?
	// HNSWWrapper exposes 'nodes' (unexported) to package.
	coll.HNSWIndex.mu.RLock()
	for id := range coll.HNSWIndex.nodes {
		if !docMapIDs[id] {
			report.OrphanIDs = append(report.OrphanIDs, id)
			report.OrphanVectors++
		}
		delete(docMapIDs, id) // Mark as found
	}
	coll.HNSWIndex.mu.RUnlock()

	// Remaining IDs in docMapIDs are missing from HNSW
	for id := range docMapIDs {
		report.MissingIDs = append(report.MissingIDs, id)
		report.MissingVectors++
	}

	return report, nil
}

// RepairOrphans removes orphan vectors from HNSW that are not in DocMap.
func (rm *RepairManager) RepairOrphans(collectionName string) error {
	report, err := rm.CheckConsistency(collectionName)
	if err != nil {
		return err
	}

	if len(report.OrphanIDs) == 0 {
		return nil // Nothing to repair
	}

	coll, err := rm.cm.GetCollection(collectionName)
	if err != nil {
		return err
	}

	coll.mu.Lock()
	defer coll.mu.Unlock()

	for _, orphanID := range report.OrphanIDs {
		if err := coll.HNSWIndex.Delete(orphanID); err != nil {
			log.Printf("Warning: failed to delete orphan vector %d: %v", orphanID, err)
		}
	}

	return coll.HNSWIndex.Save()
}

// VerifyIntegrity performs a full integrity check on a collection.
func (rm *RepairManager) VerifyIntegrity(collectionName string) error {
	report, err := rm.CheckConsistency(collectionName)
	if err != nil {
		return err
	}

	if report.OrphanVectors > 0 || report.MissingVectors > 0 {
		return fmt.Errorf("integrity check failed: %d orphans, %d missing",
			report.OrphanVectors, report.MissingVectors)
	}

	return nil
}
