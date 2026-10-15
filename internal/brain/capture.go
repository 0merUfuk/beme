package brain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CaptureInput is one proposed durable outcome. Capture is meant to be called
// at task completion (selective persistence): the protocol injected at
// session start says so, and the gate refuses everything that is not a
// distilled outcome.
type CaptureInput struct {
	Type      Type
	Project   string
	Title     string
	Statement string
	Why       string
	Next      string
	Topic     string
	Source    string // "capture" (default) | "manual"
}

// Capture validates a proposal synchronously and queues it in inbox/. It is
// deliberately lock-free and fast (unique file, atomic rename): the hot path
// of a session must never wait on maintenance. Promotion is Doctor's job.
func (v *Vault) Capture(in CaptureInput, now time.Time) (*Note, error) {
	if err := v.writable(); err != nil {
		return nil, err
	}
	if in.Source == "" {
		in.Source = "capture"
	}
	n := &Note{
		Type:       in.Type,
		Project:    in.Project,
		Title:      strings.TrimSpace(in.Title),
		Status:     StatusProposed,
		Confidence: ConfObserved,
		Topic:      strings.TrimSpace(in.Topic),
		Source:     in.Source,
		Created:    now.UTC().Format(time.RFC3339Nano),
		Statement:  strings.TrimSpace(in.Statement),
		Why:        strings.TrimSpace(in.Why),
		Next:       strings.TrimSpace(in.Next),
	}
	if !v.hasProject(n.Project) {
		known, _ := v.Projects()
		slugs := make([]string, 0, len(known))
		for _, p := range known {
			slugs = append(slugs, p.Slug)
		}
		return nil, &GateError{Violations: []Violation{{Code: "project",
			Detail: fmt.Sprintf("unknown project %q (registered: %s)", n.Project, strings.Join(slugs, ", "))}}}
	}
	if unsupportedScope(n) {
		return nil, &GateError{Violations: []Violation{{Code: "scope", Detail: "open/summary notes require a registered project; use --project <slug> (global is only for cross-project decisions/learnings)"}}}
	}
	if vs := Check(n); len(vs) > 0 {
		return nil, &GateError{Violations: vs}
	}
	var intent captureObligation
	for attempt := 0; ; attempt++ {
		if attempt == 8 {
			return nil, fmt.Errorf("could not allocate a unique capture ID")
		}
		n.ID = newID(n, now)
		n.Path = filepath.Join(v.inboxDir(), n.ID+".md")
		data, err := readableNote(n)
		if err != nil {
			return nil, err
		}
		intent, err = v.markCapture(n.Project, n.ID)
		if err != nil {
			return nil, fmt.Errorf("capture not published: cannot preserve accounting intent: %w", err)
		}
		if err := writeExclusive(n.Path, data); err != nil {
			v.cancelCapture(intent)
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return nil, err
		}
		break
	}
	if err := v.recordCapture(intent, now); err != nil {
		n.AccountingWarning = "capture accepted; accounting degraded/unknown; a later capture reconciles completed publications, but interrupted intents require inspection; do not retry this accepted capture"
	}
	v.appendLog(now, "captured", fmt.Sprintf("%s [%s] %s", n.ID, n.Project, n.Title))
	return n, nil
}

// Randomness reduces collisions; atomic no-clobber publication guarantees
// that even a collision cannot turn two successful captures into one file.
func newID(n *Note, now time.Time) string {
	var id [16]byte
	_, _ = rand.Read(id[:]) // crypto/rand.Read is infallible on supported Go platforms
	return fmt.Sprintf("%s-%s-%s-%s", n.Type.prefix(), now.UTC().Format("20060102"), slugify(n.Title, 36), hex.EncodeToString(id[:]))
}

func writeExclusive(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Hard-link a fully written same-filesystem temporary file, without
	// replacing an existing directory entry. Readers never see partial notes.
	return os.Link(name, path)
}

// Done closes an open item: it moves to archive/ with status done.
func (v *Vault) Done(id string, now time.Time) (*Note, error) {
	n, err := v.retire(id, now, func(n *Note) error {
		if n.Type != TypeOpen {
			return fmt.Errorf("%s is a %s note; only open items can be marked done (use archive)", n.ID, n.Type)
		}
		n.Status = StatusDone
		return nil
	})
	return n, err
}

// Archive retires any live note (wrong, obsolete, or no longer wanted). The
// document is kept under archive/ with the reason; nothing is deleted.
func (v *Vault) Archive(id, reason string, now time.Time) (*Note, error) {
	return v.retire(id, now, func(n *Note) error {
		n.Status = StatusArchived
		if r := strings.TrimSpace(reason); r != "" {
			n.Reason = r
		}
		return nil
	})
}

func (v *Vault) retire(id string, now time.Time, mutate func(*Note) error) (*Note, error) {
	if err := v.writable(); err != nil {
		return nil, err
	}
	unlock, err := v.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	n, err := v.Find(id)
	if err != nil {
		return nil, err
	}
	if err := mutate(n); err != nil {
		return nil, err
	}
	n.Updated = now.UTC().Format(time.RFC3339)
	src := n.Path
	if err := v.writeNote(v.archivePath(n), n); err != nil {
		return nil, err
	}
	if err := os.Remove(src); err != nil {
		return nil, err
	}
	n.Path = v.archivePath(n)
	v.appendLog(now, n.Status, fmt.Sprintf("%s [%s]", n.ID, n.Project))
	return n, nil
}
