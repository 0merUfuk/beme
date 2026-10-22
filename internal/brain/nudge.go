package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Per-session nudge bookkeeping for the Stop-hook capture reminder. Only a hash
// of the opaque harness session ID and a counter are kept, never text.
const nudgeWindow = 64

type nudgeMark struct {
	Hash      string `yaml:"h"`
	Mutations int    `yaml:"m"`
}

func nudgeKey(harness, id string) string {
	sum := sha256.Sum256([]byte(harness + "\x00" + id))
	return hex.EncodeToString(sum[:])
}

// NudgeMark returns the transcript mutation count at which this session was
// last reminded, or 0. Unreadable state reads as 0; callers must treat a failed
// SetNudgeMark as "do not remind", so a reminder can never repeat unbounded.
func (v *Vault) NudgeMark(slug, harness, id string) int {
	key := nudgeKey(harness, id)
	for _, m := range v.loadState(slug).NudgeMarks {
		if m.Hash == key {
			return m.Mutations
		}
	}
	return 0
}

// SetNudgeMark records that the session was reminded at the given count.
func (v *Vault) SetNudgeMark(slug, harness, id string, mutations int) error {
	if id == "" || len(id) > 1024 || len(harness) > 128 {
		return fmt.Errorf("session identity required")
	}
	key := nudgeKey(harness, id)
	return v.updateState(slug, func(st *projState) error {
		for i := range st.NudgeMarks {
			if st.NudgeMarks[i].Hash == key {
				st.NudgeMarks[i].Mutations = mutations
				return nil
			}
		}
		st.NudgeMarks = append(st.NudgeMarks, nudgeMark{Hash: key, Mutations: mutations})
		if len(st.NudgeMarks) > nudgeWindow {
			st.NudgeMarks = st.NudgeMarks[len(st.NudgeMarks)-nudgeWindow:]
		}
		return nil
	}, nil)
}
