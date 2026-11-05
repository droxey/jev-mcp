package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
)

// Normalize fills omitted snapshot hashes without altering caller-owned sources.
// A supplied nonempty hash is retained so Lint can reject a mismatch.
//
//nolint:gocritic // Public value API mirrors Evaluate; snapshot strings are not copied.
func Normalize(p Plan) Plan {
	p.Sources = slices.Clone(p.Sources)
	for i := range p.Sources {
		if p.Sources[i].SHA256 == "" {
			h := sha256.Sum256([]byte(p.Sources[i].Snapshot))
			p.Sources[i].SHA256 = hex.EncodeToString(h[:])
		}
	}
	return p
}

// SourceHashes returns deterministic digests of supplied snapshot bytes. It does
// not assert that a caller-supplied expected hash matches; Lint performs that check.
//
//nolint:gocritic // Public value API mirrors Lint and Evaluate.
func SourceHashes(p Plan) map[string]string {
	out := make(map[string]string, len(p.Sources))
	for i := range p.Sources {
		h := sha256.Sum256([]byte(p.Sources[i].Snapshot))
		out[p.Sources[i].ID] = hex.EncodeToString(h[:])
	}
	return out
}
