package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAggregateFromFileRetainsSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	records := []StreamRecord{
		{Package: anchorPackage, Name: anchorName, Iterations: 100, NSPerOp: 10, CapturedAt: "2026-06-01T00:00:00Z"},
		{Package: anchorPackage, Name: anchorName, Iterations: 90, NSPerOp: 20, CapturedAt: "2026-06-01T00:00:01Z"},
		{Package: "pkg", Name: "BenchmarkA", Iterations: 50, NSPerOp: 30, BytesPerOp: 100, AllocsPerOp: 1, CapturedAt: "2026-06-01T00:00:02Z"},
		{Package: "pkg", Name: "BenchmarkA", Iterations: 60, NSPerOp: 60, BytesPerOp: 200, AllocsPerOp: 3, CapturedAt: "2026-06-01T00:00:03Z"},
	}
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	baseline, err := aggregateFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.Anchor.Samples) != 2 {
		t.Fatalf("anchor samples = %d, want 2", len(baseline.Anchor.Samples))
	}
	entry := baseline.Benchmarks["pkg.BenchmarkA"]
	if entry.NSPerOp != 45 {
		t.Fatalf("ns/op = %v, want 45", entry.NSPerOp)
	}
	if entry.BytesPerOp != 150 {
		t.Fatalf("bytes/op = %d, want 150", entry.BytesPerOp)
	}
	if entry.AllocsPerOp != 2 {
		t.Fatalf("allocs/op = %d, want 2", entry.AllocsPerOp)
	}
	if len(entry.Samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(entry.Samples))
	}
	if entry.Samples[0].RatioToAnchor != 2 {
		t.Fatalf("first sample ratio = %v, want 2", entry.Samples[0].RatioToAnchor)
	}
	if entry.Samples[1].RatioToAnchor != 4 {
		t.Fatalf("second sample ratio = %v, want 4", entry.Samples[1].RatioToAnchor)
	}
}

func TestWriteBaselineWritesAtomicallyReadableJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	baseline := Baseline{
		Version:       schemaVersion,
		CapturedAt:    "2026-06-01T00:00:00Z",
		CapturedAtSHA: "abc",
		Benchmarks: map[string]BenchmarkEntry{
			"pkg.BenchmarkA": {NSPerOp: 1, RatioToAnchor: 2},
		},
	}
	if err := writeBaseline(path, baseline); err != nil {
		t.Fatal(err)
	}
	read, err := readBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if read.Benchmarks["pkg.BenchmarkA"].RatioToAnchor != 2 {
		t.Fatalf("ratio = %v, want 2", read.Benchmarks["pkg.BenchmarkA"].RatioToAnchor)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("left temporary files: %v", matches)
	}
}
