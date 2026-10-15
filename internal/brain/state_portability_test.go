package brain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func assertKnownInvocations(t *testing.T, v *Vault, want int) {
	t.Helper()
	st, err := v.readState("demo")
	if err != nil || st.AccountingUnknown || st.InvocationsSinceCapture != want || st.SessionsSinceCapture != 0 {
		t.Fatalf("successful events not accounted exactly: want=%d state=%+v err=%v", want, st, err)
	}
	// Public labels must agree with persisted state; no-ID invocations are not
	// independently identified sessions. Unknown must never disguise lost writes.
	s := v.Status()
	if s.Invocations["demo"] != want || s.SilentSession["demo"] != 0 || len(s.AccountingUnknown) != 0 {
		t.Fatalf("public accounting disagrees: want=%d status=%+v", want, s)
	}
}

func TestSessionAccountingLockedEventIsExplicitAndRetryable(t *testing.T) {
	v, _ := newVault(t)
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- v.updateState("demo", func(st *projState) error {
			close(entered)
			<-release
			st.InvocationsSinceCapture++
			return nil
		}, nil)
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatal("holder could not acquire accounting lock:", err)
	}
	started := time.Now()
	err := v.RecordSession("demo", t0)
	elapsed := time.Since(started)
	// Read while the owner is still paused. Neither the refused event nor the
	// uncommitted owner may appear in a known snapshot.
	st, readErr := v.readState("demo")
	close(release)
	ownerErr := <-done
	if err == nil || err.Error() != "session accounting locked; event not recorded" {
		t.Fatalf("contention must explicitly refuse before writing: %v", err)
	}
	if elapsed < 500*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("500ms accounting wait changed or became unbounded: %v", elapsed)
	}
	if readErr != nil || st.AccountingUnknown || st.InvocationsSinceCapture != 1 {
		t.Fatalf("refused/uncommitted event changed known state: %+v %v", st, readErr)
	}
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	assertKnownInvocations(t, v, 2)
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal("retry after release:", err)
	}
	assertKnownInvocations(t, v, 3)
}

func TestSessionAccountingReplaceWithOpenReader(t *testing.T) {
	v, _ := newVault(t)
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal(err)
	}
	path := v.stateFile("demo")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	err = v.RecordSession("demo", t0.Add(time.Second))
	// POSIX normally replaces an open file; a Windows handle opened without
	// delete sharing refuses replacement. Exercise the real OS, not a mock or
	// an unlink-then-rename fallback that can destroy the prior known state.
	if err != nil {
		var renameErr *os.LinkError
		if !errors.As(err, &renameErr) || renameErr.Op != "rename" || renameErr.New != path {
			t.Fatalf("unexpected replacement failure: %v", err)
		}
		t.Logf("native open-reader replacement explicitly refused: %v", err)
		assertKnownInvocations(t, v, 1)
		got, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(got, before) {
			t.Fatal("failed replacement changed the prior state", readErr)
		}
	} else {
		t.Log("native open-reader replacement succeeded")
		assertKnownInvocations(t, v, 2)
	}
	old, readErr := io.ReadAll(reader)
	if readErr != nil || !bytes.Equal(old, before) {
		t.Fatal("open reader saw partial or rewritten state", readErr)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		if retryErr := v.RecordSession("demo", t0.Add(time.Second)); retryErr != nil {
			t.Fatal("replacement did not recover after reader closed:", retryErr)
		}
	}
	assertKnownInvocations(t, v, 2)
	if temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tmp-*")); err != nil || len(temps) != 0 {
		t.Fatal("refused replacement leaked temporary accounting files", temps, err)
	}
}

func TestAtomicReplaceFailurePreservesDestinationAndCleansTemp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(path, "retained")
	if err := os.WriteFile(sentinel, []byte("prior destination"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := writeAtomic(path, []byte("replacement")); err == nil {
			t.Fatal("nonempty destination directory was replaced")
		}
		got, err := os.ReadFile(sentinel)
		if err != nil || string(got) != "prior destination" {
			t.Fatal("failed replacement damaged its destination", err)
		}
		if temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tmp-*")); err != nil || len(temps) != 0 {
			t.Fatal("failed replacement leaked temporary files", temps, err)
		}
	}
}

func TestSessionAccountingAcrossProcesses(t *testing.T) {
	const childRoot = "BEME_TEST_ACCOUNTING_ROOT"
	const ready = "accounting-ready\n"
	const resultPrefix = "accounting-result:"
	const eventsPerChild = 3
	if root := os.Getenv(childRoot); root != "" {
		fmt.Print(ready)
		if _, err := bufio.NewReader(os.Stdin).ReadByte(); err != nil {
			t.Fatal(err)
		}
		v := Open(root)
		results := make([]string, eventsPerChild)
		for i := range results {
			if err := v.RecordSession("demo", t0); err != nil {
				results[i] = err.Error()
			}
		}
		data, err := json.Marshal(results)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(resultPrefix + string(data))
		return
	}
	v, _ := newVault(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type child struct {
		cmd *exec.Cmd
		out *bufio.Reader
		in  io.WriteCloser
	}
	var children []child
	for i := 0; i < 4; i++ {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionAccountingAcrossProcesses$")
		cmd.Env = append(os.Environ(), childRoot+"="+v.Root)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		in, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				cmd.Process.Kill()
				cmd.Wait()
			}
		})
		children = append(children, child{cmd, bufio.NewReader(out), in})
	}
	for _, c := range children {
		if line, err := c.out.ReadString('\n'); err != nil || line != ready {
			t.Fatalf("child did not reach start barrier: %q %v", line, err)
		}
	}
	for _, c := range children {
		if _, err := io.WriteString(c.in, "g"); err != nil {
			t.Fatal(err)
		}
		c.in.Close()
	}
	refused := 0
	for _, c := range children {
		line, err := c.out.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, resultPrefix) {
			t.Fatalf("missing child accounting result: %q %v", line, err)
		}
		var results []string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, resultPrefix)), &results); err != nil || len(results) != eventsPerChild {
			t.Fatalf("invalid child results: %q %v", line, err)
		}
		for _, result := range results {
			if result == "" {
				continue
			}
			if result != "session accounting locked; event not recorded" {
				t.Fatalf("unexpected child accounting refusal: %s", result)
			}
			refused++
			t.Logf("explicit child pre-write refusal: %s", result)
		}
		if _, err := io.Copy(io.Discard, c.out); err != nil {
			t.Fatal(err)
		}
		if err := c.cmd.Wait(); err != nil {
			t.Fatal("accounting child failed:", err)
		}
	}
	assertKnownInvocations(t, v, 12-refused)
	for i := 0; i < refused; i++ {
		if err := v.RecordSession("demo", t0); err != nil {
			t.Fatal("quiescent child-event retry failed:", err)
		}
	}
	assertKnownInvocations(t, v, 12)
}
