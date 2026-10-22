package app

// Learning surfaces bound to the tombstone ledger (ADR-030). Observation files
// live in the data dir, which backups restore; the ledger's observation
// fingerprints keep restored copies of purged observations invisible on every
// learning surface (list, inspect, review, feedback dedup), and build erases
// them.

import (
	"fmt"
	"path/filepath"

	"github.com/0merUfuk/beme/internal/durable"
	"github.com/0merUfuk/beme/internal/learning"
)

// OpenLearning opens the observation store for a deployment surface. It fails
// closed (ErrLedgerUnusable) when the ledger cannot be verified, hides purged
// observations, and serializes writes with the maintenance lock.
func (rt *Runtime) OpenLearning() (*learning.Store, error) {
	ledger, err := rt.LoadLedger()
	if err != nil {
		return nil, err
	}
	return learning.OpenWith(rt.Config.DataDir, learning.Options{
		Hidden: ledger.HiddenObservation,
		Lock: func() (func(), error) {
			lk, err := rt.lock()
			if err != nil {
				return nil, err
			}
			return func() { lk.Release() }, nil
		},
	})
}

// hiddenObservationFiles lists purged observations whose files are present.
func (rt *Runtime) hiddenObservationFiles(ledger *Ledger) (*learning.Store, []string, error) {
	if len(ledger.Observations) == 0 {
		return nil, nil, nil
	}
	exists, err := durable.Exists(filepath.Join(rt.Config.DataDir, "observations"))
	if err != nil || !exists {
		return nil, nil, err
	}
	store, err := learning.OpenWith(rt.Config.DataDir, learning.Options{Hidden: ledger.HiddenObservation})
	if err != nil {
		return nil, nil, err
	}
	ids, err := store.HiddenPresent()
	return store, ids, err
}

// eraseHiddenObservations removes restored copies of purged observations.
// Callers hold the maintenance lock.
func (rt *Runtime) eraseHiddenObservations(ledger *Ledger) (int, error) {
	store, ids, err := rt.hiddenObservationFiles(ledger)
	if err != nil {
		return 0, fmt.Errorf("inspect observations: %w", err)
	}
	for _, id := range ids {
		if _, err := store.Remove(id); err != nil {
			return 0, fmt.Errorf("remove purged observation: %w", err)
		}
	}
	return len(ids), nil
}

// ObservationFindings reports doctor findings for the learning store without
// revealing observation IDs or content.
func (rt *Runtime) ObservationFindings() ([]string, error) {
	ledger, err := rt.LoadLedger()
	if err != nil {
		return nil, err
	}
	_, ids, err := rt.hiddenObservationFiles(ledger)
	if err != nil {
		return nil, fmt.Errorf("inspect observations: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return []string{fmt.Sprintf("%d purged observation file(s) restored from a backup are present (hidden on every surface): run `beme build` to erase them", len(ids))}, nil
}
