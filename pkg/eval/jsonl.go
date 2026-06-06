package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
)

// JSONLDatasetBackend loads and saves Datasets as JSONL files.
// Each line is one Example encoded as JSON.
//
// File format (one JSON object per line):
//
//	{"id":"…","input":{…},"reference":{…},"tags":["…"],"metadata":{…}}
//
// This format is compatible with most LLM eval tooling and can be generated
// from LangSmith dataset exports.
type JSONLDatasetBackend struct {
	dir string
	mem *MemoryDatasetBackend
}

// NewJSONLDatasetBackend constructs a backend that reads/writes from dir.
// dir must exist; it is not created automatically.
func NewJSONLDatasetBackend(dir string) *JSONLDatasetBackend {
	return &JSONLDatasetBackend{
		dir: dir,
		mem: NewMemoryDatasetBackend(),
	}
}

type jsonlRow struct {
	ID        uuid.UUID      `json:"id"`
	Input     map[string]any `json:"input"`
	Reference map[string]any `json:"reference,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (b *JSONLDatasetBackend) filePath(name string) string {
	return b.dir + "/" + name + ".jsonl"
}

// Push writes the dataset to "{dir}/{dataset.Name}.jsonl".
// Existing files are overwritten.
func (b *JSONLDatasetBackend) Push(_ context.Context, ds *Dataset) error {
	f, err := os.Create(b.filePath(ds.Name))
	if err != nil {
		return fmt.Errorf("jsonl backend: create %q: %w", ds.Name, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, ex := range ds.Examples {
		row := jsonlRow{
			ID:        ex.ID,
			Input:     ex.Input,
			Reference: ex.Reference,
			Tags:      ex.Tags,
			Metadata:  ex.Metadata,
		}
		if err := enc.Encode(row); err != nil {
			return fmt.Errorf("jsonl backend: encode: %w", err)
		}
	}
	return nil
}

// Pull loads a dataset from "{dir}/{name}.jsonl".
func (b *JSONLDatasetBackend) Pull(_ context.Context, name string) (*Dataset, error) {
	f, err := os.Open(b.filePath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("dataset %q not found", name)
		}
		return nil, fmt.Errorf("jsonl backend: open %q: %w", name, err)
	}
	defer f.Close()

	ds := &Dataset{ID: uuid.New(), Name: name}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var row jsonlRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("jsonl backend: decode line: %w", err)
		}
		if row.ID == uuid.Nil {
			row.ID = uuid.New()
		}
		ds.Examples = append(ds.Examples, &Example{
			ID:        row.ID,
			Input:     row.Input,
			Reference: row.Reference,
			Tags:      row.Tags,
			Metadata:  row.Metadata,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("jsonl backend: scan: %w", err)
	}
	return ds, nil
}

// PushResults appends the report summary to "{dir}/{datasetID}.results.jsonl".
func (b *JSONLDatasetBackend) PushResults(_ context.Context, report *EvalReport) error {
	path := fmt.Sprintf("%s/%s.results.jsonl", b.dir, report.DatasetID)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("jsonl backend: open results %q: %w", path, err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(report)
}

// ListDatasets lists all .jsonl files in dir as dataset names (without extension).
func (b *JSONLDatasetBackend) ListDatasets(_ context.Context) ([]*Dataset, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, fmt.Errorf("jsonl backend: readdir: %w", err)
	}
	var out []*Dataset
	for _, e := range entries {
		name := e.Name()
		if len(name) < 7 || name[len(name)-6:] != ".jsonl" {
			continue
		}
		// Exclude results files.
		base := name[:len(name)-6]
		if len(base) > 8 && base[len(base)-8:] == ".results" {
			continue
		}
		out = append(out, &Dataset{Name: base})
	}
	return out, nil
}
