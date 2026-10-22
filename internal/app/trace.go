package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
)

const maxRetainedTraces = 200

var traceMu sync.Mutex

// TraceBinding records policy applicability, not task text or machine paths.
// It allows explain to reapply current eligibility without inferring authority.
type TraceBinding struct {
	Profile     contracts.Profile `json:"profile"`
	WorkspaceID string            `json:"workspace_id,omitempty"`
	TaskKinds   []string          `json:"task_kinds,omitempty"`
	Risk        string            `json:"risk,omitempty"`
	Learned     bool              `json:"learned,omitempty"`
}

type storedTrace struct {
	Binding TraceBinding         `json:"binding"`
	Steps   []resolver.TraceStep `json:"steps"`
}

func safeTrace(steps []resolver.TraceStep) []resolver.TraceStep {
	eligible := map[string]bool{}
	for _, st := range steps {
		if st.Step == "stage_a" && st.Outcome == "eligible" {
			eligible[st.RecordID] = true
		}
	}
	out := []resolver.TraceStep{}
	for _, st := range steps {
		if st.RecordID != "" && !eligible[st.RecordID] {
			continue
		}
		out = append(out, st)
	}
	return out
}

func (s *Session) recordTrace(pack *resolver.Pack, steps []resolver.TraceStep, tc policy.TaskContext) {
	binding := TraceBinding{Profile: s.Capability.Profile, WorkspaceID: tc.WorkspaceID, TaskKinds: tc.TaskKinds, Risk: tc.Risk, Learned: s.Capability.ExperimentalLearnedGuidance}
	if err := PersistTrace(s.Runtime.Config.CacheDir, pack.TraceRef, steps, binding); err != nil {
		pack.TraceRef = ""
		pack.Degradations = append(pack.Degradations, resolver.PackDegradation{Kind: "reduced_assurance", Detail: "resolution trace could not be persisted; explain is unavailable for this result (context reads unaffected)"})
		pack.Resolution.Completeness = "incomplete"
		pack.RecountTokens()
	}
}

// PersistTrace is an immutable, bounded cache write shared by serving and the
// legacy CLI callback. Already persisted traces are never rewritten. Failure
// is explicit to the serving boundary, but cannot make context reads fail.
// The optional binding preserves source compatibility with the CLI callback;
// all normal serving writes include it. Only sanitized steps may be passed.
func PersistTrace(cacheDir, traceRef string, steps []resolver.TraceStep, binding ...TraceBinding) error {
	id, ok := traceFileID(traceRef)
	if !ok {
		return ErrTraceUnavailable
	}
	traceMu.Lock()
	defer traceMu.Unlock()
	dir := filepath.Join(cacheDir, "traces")
	target := filepath.Join(dir, id+".json")
	if info, err := os.Stat(target); err == nil {
		if info.Mode().IsRegular() {
			return nil
		}
		return ErrTraceUnavailable
	} else if !os.IsNotExist(err) {
		return err
	}
	var payload any = steps
	if len(binding) > 0 {
		payload = storedTrace{Binding: binding[0], Steps: steps}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".trace-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, target); err != nil {
		return err
	}
	if err = pruneTraces(dir, id+".json", maxRetainedTraces); err != nil {
		return err
	}
	actual, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, data) {
		return fmt.Errorf("trace cache verification failed")
	}
	return nil
}

func traceFileID(ref string) (string, bool) {
	id := strings.TrimPrefix(ref, "trace_")
	if len(id) == 0 || len(id) > 128 {
		return "", false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return "", false
		}
	}
	return id, true
}

func pruneTraces(dir, current string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type age struct {
		name     string
		modified time.Time
	}
	files := []age{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		files = append(files, age{entry.Name(), info.ModTime()})
	}
	// IDs are random, not chronological. Protect the trace just issued even on
	// coarse clocks; prune by modification time, with a deterministic tie break.
	sort.Slice(files, func(i, j int) bool {
		if files[i].name == current {
			return true
		}
		if files[j].name == current {
			return false
		}
		if !files[i].modified.Equal(files[j].modified) {
			return files[i].modified.After(files[j].modified)
		}
		return files[i].name < files[j].name
	})
	for i := keep; i < len(files); i++ {
		if err := os.Remove(filepath.Join(dir, files[i].name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
