package brain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// These are delivery-invocation and accepted-capture counters, not use or
// benefit measurements. Identified starts are duplicate-safe within a bounded
// 512-ID window. Only hashes of opaque harness IDs are retained, never text.
const sessionIDWindow = 512

type projState struct {
	SessionsSinceCapture    int      `yaml:"identified_sessions_since_capture,omitempty"`
	InvocationsSinceCapture int      `yaml:"invocations_since_capture,omitempty"`
	LegacyInvocations       int      `yaml:"sessions_since_capture,omitempty"`
	SeenSessions            []string `yaml:"seen_session_hashes,omitempty"`
	LastSession             string   `yaml:"last_session,omitempty"`
	LastCapture             string   `yaml:"last_capture,omitempty"`
	CaptureToken            string   `yaml:"capture_token,omitempty"` // Legacy shared-marker acknowledgement only.
	CaptureEpoch            uint64   `yaml:"capture_epoch,omitempty"`
	AccountingUnknown       bool     `yaml:"-"`
}

func (v *Vault) stateFile(slug string) string { return filepath.Join(v.Root, ".state", slug+".yaml") }
func slugOK(slug string) bool                 { return slug == GlobalProject || slugRe.MatchString(slug) }

func (v *Vault) readState(slug string) (projState, error) {
	var st projState
	if !slugOK(slug) {
		return st, fmt.Errorf("invalid project scope")
	}
	data, err := v.readFile(v.stateFile(slug))
	if err != nil && !os.IsNotExist(err) {
		return st, err
	}
	if err == nil {
		if err := yaml.Unmarshal(data, &st); err != nil {
			return st, err
		}
	}
	marker, markerErr := v.readFile(v.captureMarker(slug))
	st.AccountingUnknown = (markerErr != nil && !os.IsNotExist(markerErr)) || (markerErr == nil && strings.TrimSpace(string(marker)) != st.CaptureToken)
	if len(marker) == 0 && markerErr == nil {
		st.AccountingUnknown = true
	}
	// Any occupied slot (including empty, partial or refused intent) is an
	// independent obligation. Readers never reconcile or repair bookkeeping.
	for slot := 0; slot < captureIntentLimit && !st.AccountingUnknown; slot++ {
		_, err := checkPath(v.Root, v.captureIntentPath(slug, slot))
		st.AccountingUnknown = !os.IsNotExist(err)
	}
	// Do not combine counters read before a reset with slots cleared after it.
	// Every reset increments the epoch; changed snapshots fail conservatively.
	if !st.AccountingUnknown {
		after, err := v.readFile(v.stateFile(slug))
		st.AccountingUnknown = (err != nil && !os.IsNotExist(err)) || !bytes.Equal(data, after)
	}
	// V1 counted invocations without IDs, not distinct sessions.
	st.InvocationsSinceCapture += st.LegacyInvocations
	st.LegacyInvocations = 0
	return st, nil
}
func (v *Vault) loadState(slug string) projState { st, _ := v.readState(slug); return st }

// Serialize read-modify-write across processes separately from maintenance.
// Post-commit cleanup stays under this same lock, after state publication.
// Capture never waits for Doctor; bookkeeping contention is bounded and
// reported by the explicit event path, while the accepted proposal survives.
func (v *Vault) updateState(slug string, change func(*projState) error, committed func() error) error {
	if !slugOK(slug) || !v.hasProject(slug) {
		return fmt.Errorf("unknown project scope")
	}
	if err := v.writable(); err != nil {
		return err
	}
	dir := filepath.Join(v.Root, ".state")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if _, err := checkPath(v.Root, dir); err != nil {
		return err
	}
	lock := filepath.Join(dir, slug+".lock")
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
			break
		}
		if !os.IsExist(err) {
			return err
		}
		if st, err := os.Lstat(lock); err == nil && st.Mode().IsRegular() && time.Since(st.ModTime()) > time.Minute {
			if err := os.Remove(lock); err == nil {
				continue
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session accounting locked; event not recorded")
		}
		time.Sleep(time.Millisecond)
	}
	defer os.Remove(lock)
	if err := v.writable(); err != nil {
		return err
	}
	st, err := v.readState(slug)
	if err != nil {
		return err
	}
	if err := change(&st); err != nil {
		return err
	}
	data, err := yaml.Marshal(st)
	if err != nil {
		return err
	}
	if err := writeAtomic(v.stateFile(slug), data); err != nil {
		return err
	}
	if committed != nil {
		return committed()
	}
	return nil
}

// RecordSession without an opaque ID records ONLY an invocation.
func (v *Vault) RecordSession(slug string, now time.Time) error {
	return v.RecordSessionEvent(slug, "", "", now)
}
func (v *Vault) RecordSessionEvent(slug, harness, id string, now time.Time) error {
	if slug == "" {
		return nil
	}
	if len(id) > 1024 || len(harness) > 128 {
		return fmt.Errorf("session identity exceeds accounting bound")
	}
	return v.updateState(slug, func(st *projState) error {
		st.InvocationsSinceCapture++
		st.LastSession = now.UTC().Format(time.RFC3339Nano)
		if id == "" {
			return nil
		}
		sum := sha256.Sum256([]byte(harness + "\x00" + id))
		key := hex.EncodeToString(sum[:])
		for _, seen := range st.SeenSessions {
			if key == seen {
				return nil
			}
		}
		st.SessionsSinceCapture++
		st.SeenSessions = append(st.SeenSessions, key)
		if len(st.SeenSessions) > sessionIDWindow {
			st.SeenSessions = st.SeenSessions[len(st.SeenSessions)-sessionIDWindow:]
		}
		return nil
	}, nil)
}

func (v *Vault) captureMarker(slug string) string {
	return filepath.Join(v.Root, ".state", slug+".capture")
}

// Fixed slots bound live bookkeeping independently of lifetime capture count.
// Neither age nor another publisher can acknowledge a pending intent. A
// partial/interrupted intent consumes a slot and remains unknown until inspected.
const captureIntentLimit = 128

type captureObligation struct {
	slug, id, path string
	file           os.FileInfo // Identifies this allocation even after slot/ID reuse.
}

func (v *Vault) captureIntentPath(slug string, slot int) string {
	return filepath.Join(v.Root, ".state", fmt.Sprintf("%s.capture-%03d", slug, slot))
}
func (v *Vault) markCapture(slug, id string) (captureObligation, error) {
	var intent captureObligation
	if !slugOK(slug) || id == "" || len(id) > 256 || strings.ContainsAny(id, "\r\n") {
		return intent, fmt.Errorf("invalid capture accounting identity")
	}
	dir := filepath.Dir(v.captureMarker(slug))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return intent, err
	}
	if _, err := checkPath(v.Root, dir); err != nil {
		return intent, err
	}
	for slot := 0; slot < captureIntentLimit; slot++ {
		path := v.captureIntentPath(slug, slot)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return intent, err
		}
		info, err := f.Stat()
		if err == nil {
			_, err = f.WriteString(id + "\n")
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			os.Remove(path) // This failed attempt cannot publish a proposal.
			if err != nil {
				return intent, err
			}
			return intent, closeErr
		}
		return captureObligation{slug: slug, id: id, path: path, file: info}, nil
	}
	return intent, fmt.Errorf("capture accounting intent capacity reached (%d); inspect interrupted captures", captureIntentLimit)
}

// Only the owner cancels an unpublished attempt (including a note-ID collision).
// Resets cannot remove pending slots. Failed cancellation stays unknown.
func (v *Vault) cancelCapture(intent captureObligation) {
	info, err := checkPath(v.Root, intent.path)
	if err != nil || !os.SameFile(info, intent.file) {
		return
	}
	data, err := readRegular(v.Root, intent.path, 512)
	if err == nil && string(data) == intent.id+"\n" {
		os.Remove(intent.path)
	}
}

// Append completion only AFTER publication and only once. Until this append
// finishes no reset can reclaim the slot. Afterwards its owner must never
// write this path again: another reset may reclaim and reuse it while we wait.
func (v *Vault) recordCapture(intent captureObligation, now time.Time) error {
	info, err := checkPath(v.Root, intent.path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, intent.file) {
		return fmt.Errorf("capture accounting intent changed")
	}
	data, err := readRegular(v.Root, intent.path, 512)
	if err != nil {
		return err
	}
	if string(data) != intent.id+"\n" {
		return fmt.Errorf("capture accounting intent invalid")
	}
	f, err := os.OpenFile(intent.path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString("published\n")
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return v.resetCaptures(intent, now)
}

// Snapshot completed obligations under the accounting lock; remove them only
// AFTER state commit, still under that lock. In-flight publications remain.
// If a newer reset already included our receipt, do not reset newer sessions.
func (v *Vault) resetCaptures(intent captureObligation, now time.Time) error {
	var completed []string
	return v.updateState(intent.slug, func(st *projState) error {
		info, err := checkPath(v.Root, intent.path)
		if os.IsNotExist(err) || (err == nil && !os.SameFile(info, intent.file)) {
			return nil
		}
		if err != nil {
			return err
		}
		own, err := readRegular(v.Root, intent.path, 512)
		if err != nil {
			return err
		}
		if string(own) != intent.id+"\npublished\n" {
			return fmt.Errorf("capture publication not complete")
		}
		for slot := 0; slot < captureIntentLimit; slot++ {
			path := v.captureIntentPath(intent.slug, slot)
			data, err := readRegular(v.Root, path, 512)
			parts := strings.Split(string(data), "\n")
			if err == nil && len(parts) == 3 && parts[0] != "" && parts[1] == "published" && parts[2] == "" {
				completed = append(completed, path)
			}
		}
		if st.CaptureEpoch == ^uint64(0) {
			return fmt.Errorf("capture accounting epoch exhausted")
		}
		st.SessionsSinceCapture, st.InvocationsSinceCapture = 0, 0
		if previous, err := time.Parse(time.RFC3339Nano, st.LastCapture); err != nil || now.After(previous) {
			st.LastCapture = now.UTC().Format(time.RFC3339Nano)
		}
		// Leave legacy CaptureToken untouched: a new capture cannot prove that an
		// unknown legacy marker (possibly from an old process) is reconciled.
		st.CaptureEpoch++
		return nil
	}, func() error {
		for _, path := range completed {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	})
}
