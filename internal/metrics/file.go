package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// FileSource reads pre-collected usage records from a JSON file. It exists so
// the tool can produce a real report offline — for demos, CI fixtures, or
// feeding in metrics exported from somewhere else — using the exact same
// engine as the live Prometheus path.
type FileSource struct {
	Path string
}

// Collect loads the file and applies the selector's namespace filter.
func (f FileSource) Collect(_ context.Context, sel Selector) ([]ContainerUsage, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, fmt.Errorf("read usage file: %w", err)
	}
	var all []ContainerUsage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, fmt.Errorf("parse usage file %s: %w", f.Path, err)
	}
	if sel.Namespace == "" {
		return all, nil
	}
	out := all[:0]
	for _, u := range all {
		if u.Namespace == sel.Namespace {
			out = append(out, u)
		}
	}
	return out, nil
}
