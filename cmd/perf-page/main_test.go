package main

import "testing"

func TestSplitBenchmarkName(t *testing.T) {
	pkg, name := splitBenchmarkName("github.com/nooga/let-go/pkg/vm.BenchmarkFuncInvoke/Direct")
	if pkg != "pkg/vm" {
		t.Fatalf("package = %q, want pkg/vm", pkg)
	}
	if name != "BenchmarkFuncInvoke/Direct" {
		t.Fatalf("name = %q, want BenchmarkFuncInvoke/Direct", name)
	}

	pkg, name = splitBenchmarkName("github.com/nooga/let-go/pkg/ir.BenchmarkIRCompile [gogen_ir]")
	if pkg != "pkg/ir" {
		t.Fatalf("package = %q, want pkg/ir", pkg)
	}
	if name != "BenchmarkIRCompile [gogen_ir]" {
		t.Fatalf("name = %q, want BenchmarkIRCompile [gogen_ir]", name)
	}
}

func TestCompareWithHistorical(t *testing.T) {
	current := Baseline{Benchmarks: map[string]BenchmarkEntry{
		"pkg.BenchmarkA": {Ratio: 80},
		"pkg.BenchmarkB": {Ratio: 120},
		"pkg.BenchmarkC": {Ratio: 50},
	}}
	reference := Baseline{Benchmarks: map[string]BenchmarkEntry{
		"pkg.BenchmarkA": {Ratio: 100},
		"pkg.BenchmarkB": {Ratio: 100},
		"pkg.BenchmarkD": {Ratio: 10},
	}}

	changes, summary := compare(current, reference)
	if summary.Common != 2 {
		t.Fatalf("common = %d, want 2", summary.Common)
	}
	if summary.New != 1 {
		t.Fatalf("new = %d, want 1", summary.New)
	}
	if summary.Missing != 1 {
		t.Fatalf("missing = %d, want 1", summary.Missing)
	}
	if summary.Faster != 1 {
		t.Fatalf("faster = %d, want 1", summary.Faster)
	}
	if summary.Slower != 1 {
		t.Fatalf("slower = %d, want 1", summary.Slower)
	}
	if summary.MedianDelta != 0 {
		t.Fatalf("median delta = %v, want 0", summary.MedianDelta)
	}
	if got := changes["pkg.BenchmarkA"]; !near(got, -0.2) {
		t.Fatalf("BenchmarkA delta = %v, want -0.2", got)
	}
	if got := changes["pkg.BenchmarkB"]; !near(got, 0.2) {
		t.Fatalf("BenchmarkB delta = %v, want 0.2", got)
	}
}

func TestBuildCharts(t *testing.T) {
	timeline := []Snapshot{
		makeSnapshot("a", Baseline{
			CapturedAt:    "2026-06-01T00:00:00Z",
			CapturedAtSHA: "aaaaaaaaaaaa",
			Benchmarks: map[string]BenchmarkEntry{
				"github.com/nooga/let-go/test.BenchmarkClojureTestSuite [bytecode]": {Ratio: 100, AllocsPerOp: 10, BytesPerOp: 1000},
			},
		}),
		makeSnapshot("b", Baseline{
			CapturedAt:    "2026-06-02T00:00:00Z",
			CapturedAtSHA: "bbbbbbbbbbbb",
			Benchmarks: map[string]BenchmarkEntry{
				"github.com/nooga/let-go/test.BenchmarkClojureTestSuite [bytecode]": {Ratio: 80, AllocsPerOp: 9, BytesPerOp: 900},
			},
		}),
	}

	charts := buildCharts(timeline)
	if len(charts) != 3 {
		t.Fatalf("chart count = %d, want 3", len(charts))
	}
	if charts[0].Title != "End-to-end suite" {
		t.Fatalf("first chart = %q, want End-to-end suite", charts[0].Title)
	}
	if len(charts[0].Series) != 1 {
		t.Fatalf("series count = %d, want 1", len(charts[0].Series))
	}
	if len(charts[0].Series[0].Points) != 2 {
		t.Fatalf("point count = %d, want 2", len(charts[0].Series[0].Points))
	}
	if charts[0].Series[0].Path == "" {
		t.Fatal("expected SVG path")
	}
	if charts[0].Series[0].Points[1].X <= charts[0].Series[0].Points[0].X {
		t.Fatalf("x coordinates did not advance: %#v", charts[0].Series[0].Points)
	}
}

func near(got, want float64) bool {
	diff := got - want
	return diff < 0.0000001 && diff > -0.0000001
}

func TestFormatNS(t *testing.T) {
	tests := map[float64]string{
		12.345:         "12.35 ns",
		12_345:         "12.35 us",
		12_345_000:     "12.35 ms",
		12_345_000_000: "12.35 s",
	}
	for input, want := range tests {
		if got := formatNS(input); got != want {
			t.Fatalf("formatNS(%v) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatRatioCompactsLargeValues(t *testing.T) {
	tests := map[float64]string{
		1_124_520_183: "1.12B",
		8_519_621:     "8.52M",
		66_309:        "66.3k",
		16.209:        "16.2",
		4.688:         "4.69",
	}
	for input, want := range tests {
		if got := formatRatio(input); got != want {
			t.Fatalf("formatRatio(%v) = %q, want %q", input, got, want)
		}
	}
}
