package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nooga/let-go/pkg/perfdata"
	"golang.org/x/perf/benchunit"
)

const modulePrefix = "github.com/nooga/let-go/"

type Baseline = perfdata.Baseline
type Machine = perfdata.Machine
type Anchor = perfdata.Anchor
type BenchmarkEntry = perfdata.BenchmarkEntry

type BenchmarkRow struct {
	FullName     string
	Package      string
	Name         string
	NSPerOp      float64
	AllocsPerOp  float64
	BytesPerOp   float64
	Ratio        float64
	BestSinceSHA string
	BestSinceAt  string
	Delta        *float64
	BarWidth     float64
}

type ChangeRow struct {
	BenchmarkRow
	OldRatio float64
	NewRatio float64
}

type Snapshot struct {
	Name     string
	Baseline Baseline
	Captured time.Time
}

type Chart struct {
	Title    string
	Subtitle string
	Unit     string
	Series   []ChartSeries
	YMin     float64
	YMax     float64
	YMinText string
	YMaxText string
}

type ChartSeries struct {
	Label  string
	Color  string
	Path   string
	Points []ChartPoint
}

type ChartPoint struct {
	X     float64
	Y     float64
	Index int
	Date  string
	SHA   string
	Value float64
	Text  string
}

type Summary struct {
	BenchmarkCount int
	PackageCount   int
	ZeroAllocs     int
	Common         int
	New            int
	Missing        int
	Faster         int
	Slower         int
	MedianDelta    float64
}

type PageData struct {
	Title             string
	LogoDataURI       template.URL
	Current           Baseline
	ReferenceName     string
	Summary           Summary
	Timeline          []Snapshot
	Charts            []Chart
	Rows              []BenchmarkRow
	TopImprovements   []ChangeRow
	TopSlowdowns      []ChangeRow
	RecentlyTightened []BenchmarkRow
}

func main() {
	var (
		baselinePath   = flag.String("baseline", "docs/perf/baseline.json", "current baseline JSON")
		historicalPath = flag.String("historical", "docs/perf/historical", "historical baseline directory")
		timelinePath   = flag.String("timeline", "docs/perf/timeline", "timeline snapshot directory")
		outPath        = flag.String("out", "docs/perf/index.html", "HTML output path")
		logoPath       = flag.String("logo", "meta/logo.svg", "logo SVG to embed")
	)
	flag.Parse()

	current, err := loadBaseline(*baselinePath)
	if err != nil {
		die("load baseline: %v", err)
	}
	reference, referenceName, err := loadLatestHistorical(*historicalPath)
	if err != nil {
		die("load historical baseline: %v", err)
	}
	timeline, err := loadTimeline(*timelinePath, *historicalPath, current)
	if err != nil {
		die("load timeline: %v", err)
	}
	logo, err := logoDataURI(*logoPath)
	if err != nil {
		die("load logo: %v", err)
	}

	page := buildPage(current, reference, referenceName, timeline, logo)
	html, err := renderPage(page)
	if err != nil {
		die("render page: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		die("create output directory: %v", err)
	}
	if err := os.WriteFile(*outPath, html, 0o644); err != nil {
		die("write %s: %v", *outPath, err)
	}
	fmt.Printf("wrote %s (%d benchmarks)\n", *outPath, len(current.Benchmarks))
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "perf-page: "+format+"\n", args...)
	os.Exit(1)
}

func loadBaseline(path string) (Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, err
	}
	var baseline Baseline
	if err := json.Unmarshal(b, &baseline); err != nil {
		return Baseline{}, err
	}
	if len(baseline.Benchmarks) == 0 {
		return Baseline{}, fmt.Errorf("%s has no benchmarks", path)
	}
	return baseline, nil
}

func loadLatestHistorical(dir string) (Baseline, string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return Baseline{}, "", err
	}
	if len(matches) == 0 {
		return Baseline{}, "", nil
	}

	type historical struct {
		path     string
		name     string
		baseline Baseline
		captured time.Time
	}
	var all []historical
	for _, match := range matches {
		baseline, err := loadBaseline(match)
		if err != nil {
			warnSkipBaseline(match, err)
			continue
		}
		captured, _ := time.Parse(time.RFC3339, baseline.CapturedAt)
		name := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		all = append(all, historical{
			path:     match,
			name:     name,
			baseline: baseline,
			captured: captured,
		})
	}
	if len(all) == 0 {
		return Baseline{}, "", nil
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].captured.Equal(all[j].captured) {
			return all[i].captured.After(all[j].captured)
		}
		return all[i].path > all[j].path
	})
	return all[0].baseline, all[0].name, nil
}

func loadTimeline(timelineDir, historicalDir string, current Baseline) ([]Snapshot, error) {
	var snapshots []Snapshot
	matches, err := filepath.Glob(filepath.Join(timelineDir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		baseline, err := loadBaseline(match)
		if err != nil {
			warnSkipBaseline(match, err)
			continue
		}
		snapshots = append(snapshots, makeSnapshot(strings.TrimSuffix(filepath.Base(match), filepath.Ext(match)), baseline))
	}
	if len(snapshots) == 0 {
		historical, err := filepath.Glob(filepath.Join(historicalDir, "*.json"))
		if err != nil {
			return nil, err
		}
		for _, match := range historical {
			baseline, err := loadBaseline(match)
			if err != nil {
				warnSkipBaseline(match, err)
				continue
			}
			snapshots = append(snapshots, makeSnapshot(strings.TrimSuffix(filepath.Base(match), filepath.Ext(match)), baseline))
		}
		snapshots = append(snapshots, makeSnapshot("current-ratchet", current))
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if !snapshots[i].Captured.Equal(snapshots[j].Captured) {
			return snapshots[i].Captured.Before(snapshots[j].Captured)
		}
		if snapshots[i].Baseline.CapturedAtSHA != snapshots[j].Baseline.CapturedAtSHA {
			return snapshots[i].Baseline.CapturedAtSHA < snapshots[j].Baseline.CapturedAtSHA
		}
		return snapshots[i].Name < snapshots[j].Name
	})
	return snapshots, nil
}

func warnSkipBaseline(path string, err error) {
	fmt.Fprintf(os.Stderr, "perf-page: warning: skipping %s: %v\n", path, err)
}

func makeSnapshot(name string, baseline Baseline) Snapshot {
	captured, _ := time.Parse(time.RFC3339, baseline.CapturedAt)
	return Snapshot{Name: name, Baseline: baseline, Captured: captured}
}

func logoDataURI(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	return "data:image/svg+xml;base64," + encoded, nil
}

func buildPage(current, reference Baseline, referenceName string, timeline []Snapshot, logo string) PageData {
	rows := benchmarkRows(current)
	changes, summary := compare(current, reference)
	maxRatio := 0.0
	packageSet := map[string]struct{}{}
	for i := range rows {
		if rows[i].Ratio > maxRatio {
			maxRatio = rows[i].Ratio
		}
		packageSet[rows[i].Package] = struct{}{}
		if delta, ok := changes[rows[i].FullName]; ok {
			rows[i].Delta = &delta
		}
		if rows[i].AllocsPerOp == 0 {
			summary.ZeroAllocs++
		}
	}
	for i := range rows {
		rows[i].BarWidth = barWidth(rows[i].Ratio, maxRatio)
	}
	summary.BenchmarkCount = len(rows)
	summary.PackageCount = len(packageSet)

	recent := append([]BenchmarkRow(nil), rows...)
	sort.Slice(recent, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, recent[i].BestSinceAt)
		tj, _ := time.Parse(time.RFC3339, recent[j].BestSinceAt)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return recent[i].FullName < recent[j].FullName
	})
	if len(recent) > 8 {
		recent = recent[:8]
	}

	improvements, slowdowns := topChanges(current, reference, 8)
	return PageData{
		Title:             "Are we fast yet?",
		LogoDataURI:       template.URL(logo),
		Current:           current,
		ReferenceName:     referenceName,
		Summary:           summary,
		Timeline:          timeline,
		Charts:            buildCharts(timeline),
		Rows:              rows,
		TopImprovements:   improvements,
		TopSlowdowns:      slowdowns,
		RecentlyTightened: recent,
	}
}

func buildCharts(timeline []Snapshot) []Chart {
	specs := []struct {
		title    string
		subtitle string
		unit     string
		metric   func(BenchmarkEntry) float64
		format   func(float64) string
		series   []chartSeriesSpec
	}{
		{
			title:    "End-to-end suite",
			subtitle: "Anchor-normalized jank clojure-test-suite wall time. Lower is better.",
			unit:     "anchors",
			metric:   func(entry BenchmarkEntry) float64 { return entry.RatioToAnchor },
			format:   formatRatio,
			series: []chartSeriesSpec{
				{label: "bytecode", color: "#245c73", name: "github.com/nooga/let-go/test.BenchmarkClojureTestSuite [bytecode]"},
				{label: "gogen_ir", color: "#167a48", name: "github.com/nooga/let-go/test.BenchmarkClojureTestSuite [gogen_ir]"},
			},
		},
		{
			title:    "IR compile",
			subtitle: "Anchor-normalized IR compile benchmark variants. Lower is better.",
			unit:     "anchors",
			metric:   func(entry BenchmarkEntry) float64 { return entry.RatioToAnchor },
			format:   formatRatio,
			series: []chartSeriesSpec{
				{label: "bytecode", color: "#245c73", name: "github.com/nooga/let-go/pkg/ir.BenchmarkIRCompile [bytecode]"},
				{label: "gogen_ir", color: "#167a48", name: "github.com/nooga/let-go/pkg/ir.BenchmarkIRCompile [gogen_ir]"},
			},
		},
		{
			title:    "Suite allocations",
			subtitle: "allocs/op for the full suite benchmark variants. Lower is better.",
			unit:     "allocs/op",
			metric:   func(entry BenchmarkEntry) float64 { return float64(entry.AllocsPerOp) },
			format:   formatCount,
			series: []chartSeriesSpec{
				{label: "bytecode", color: "#245c73", name: "github.com/nooga/let-go/test.BenchmarkClojureTestSuite [bytecode]"},
				{label: "gogen_ir", color: "#167a48", name: "github.com/nooga/let-go/test.BenchmarkClojureTestSuite [gogen_ir]"},
			},
		},
		{
			title:    "Suite memory",
			subtitle: "bytes/op for the full suite benchmark variants. Lower is better.",
			unit:     "B/op",
			metric:   func(entry BenchmarkEntry) float64 { return float64(entry.BytesPerOp) },
			format:   formatBytes,
			series: []chartSeriesSpec{
				{label: "bytecode", color: "#245c73", name: "github.com/nooga/let-go/test.BenchmarkClojureTestSuite [bytecode]"},
				{label: "gogen_ir", color: "#167a48", name: "github.com/nooga/let-go/test.BenchmarkClojureTestSuite [gogen_ir]"},
			},
		},
	}

	charts := make([]Chart, 0, len(specs))
	for _, spec := range specs {
		chart := buildChart(timeline, spec.title, spec.subtitle, spec.unit, spec.metric, spec.format, spec.series)
		if len(chart.Series) > 0 {
			charts = append(charts, chart)
		}
	}
	return charts
}

type chartSeriesSpec struct {
	label string
	color string
	name  string
}

func buildChart(timeline []Snapshot, title, subtitle, unit string, metric func(BenchmarkEntry) float64, format func(float64) string, specs []chartSeriesSpec) Chart {
	const (
		left   = 46.0
		right  = 18.0
		top    = 22.0
		bottom = 34.0
		width  = 520.0
		height = 210.0
	)
	plotW := width - left - right
	plotH := height - top - bottom
	yMin := math.Inf(1)
	yMax := math.Inf(-1)
	series := make([]ChartSeries, 0, len(specs))

	for _, spec := range specs {
		var raw []struct {
			index int
			snap  Snapshot
			value float64
		}
		for i, snap := range timeline {
			entry, ok := snap.Baseline.Benchmarks[spec.name]
			if !ok {
				continue
			}
			value := metric(entry)
			if value <= 0 {
				continue
			}
			raw = append(raw, struct {
				index int
				snap  Snapshot
				value float64
			}{index: i, snap: snap, value: value})
			if value < yMin {
				yMin = value
			}
			if value > yMax {
				yMax = value
			}
		}
		if len(raw) == 0 {
			continue
		}
		series = append(series, ChartSeries{
			Label:  spec.label,
			Color:  spec.color,
			Points: make([]ChartPoint, 0, len(raw)),
		})
		for _, item := range raw {
			series[len(series)-1].Points = append(series[len(series)-1].Points, ChartPoint{
				Index: item.index,
				Date:  formatDate(item.snap.Baseline.CapturedAt),
				SHA:   shortSHA(item.snap.Baseline.CapturedAtSHA),
				Value: item.value,
				Text:  format(item.value),
			})
		}
	}

	if len(series) == 0 {
		return Chart{}
	}
	if yMin == yMax {
		yMin *= 0.95
		yMax *= 1.05
		if yMin == yMax {
			yMin = 0
			yMax = 1
		}
	}
	if yMin > 0 {
		padding := (yMax - yMin) * 0.08
		yMin -= padding
		yMax += padding
	}
	for si := range series {
		pathParts := make([]string, 0, len(series[si].Points))
		for pi := range series[si].Points {
			point := &series[si].Points[pi]
			denom := float64(maxInt(len(timeline)-1, 1))
			point.X = left + (float64(point.Index)/denom)*plotW
			point.Y = top + ((yMax-point.Value)/(yMax-yMin))*plotH
			cmd := "L"
			if pi == 0 {
				cmd = "M"
			}
			pathParts = append(pathParts, fmt.Sprintf("%s %.2f %.2f", cmd, point.X, point.Y))
		}
		series[si].Path = strings.Join(pathParts, " ")
	}
	return Chart{
		Title:    title,
		Subtitle: subtitle,
		Unit:     unit,
		Series:   series,
		YMin:     yMin,
		YMax:     yMax,
		YMinText: format(yMin),
		YMaxText: format(yMax),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func benchmarkRows(baseline Baseline) []BenchmarkRow {
	rows := make([]BenchmarkRow, 0, len(baseline.Benchmarks))
	for fullName, entry := range baseline.Benchmarks {
		pkg, name := splitBenchmarkName(fullName)
		rows = append(rows, BenchmarkRow{
			FullName:     fullName,
			Package:      pkg,
			Name:         name,
			NSPerOp:      entry.NSPerOp,
			AllocsPerOp:  float64(entry.AllocsPerOp),
			BytesPerOp:   float64(entry.BytesPerOp),
			Ratio:        entry.RatioToAnchor,
			BestSinceSHA: entry.BestSinceSHA,
			BestSinceAt:  entry.BestSinceAt,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Package != rows[j].Package {
			return rows[i].Package < rows[j].Package
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func splitBenchmarkName(fullName string) (string, string) {
	name := strings.TrimPrefix(fullName, modulePrefix)
	idx := strings.Index(name, ".Benchmark")
	if idx < 0 {
		return "", name
	}
	return name[:idx], name[idx+1:]
}

func compare(current, reference Baseline) (map[string]float64, Summary) {
	changes := make(map[string]float64)
	if len(reference.Benchmarks) == 0 {
		return changes, Summary{New: len(current.Benchmarks)}
	}
	var deltas []float64
	var summary Summary
	for name, cur := range current.Benchmarks {
		ref, ok := reference.Benchmarks[name]
		if !ok {
			summary.New++
			continue
		}
		if ref.RatioToAnchor == 0 {
			continue
		}
		delta := cur.RatioToAnchor/ref.RatioToAnchor - 1
		changes[name] = delta
		deltas = append(deltas, delta)
		summary.Common++
		if delta <= -0.05 {
			summary.Faster++
		} else if delta >= 0.05 {
			summary.Slower++
		}
	}
	for name := range reference.Benchmarks {
		if _, ok := current.Benchmarks[name]; !ok {
			summary.Missing++
		}
	}
	summary.MedianDelta = median(deltas)
	return changes, summary
}

func topChanges(current, reference Baseline, limit int) ([]ChangeRow, []ChangeRow) {
	if len(reference.Benchmarks) == 0 {
		return nil, nil
	}
	var rows []ChangeRow
	for _, row := range benchmarkRows(current) {
		cur := current.Benchmarks[row.FullName]
		ref, ok := reference.Benchmarks[row.FullName]
		if !ok || ref.RatioToAnchor == 0 {
			continue
		}
		delta := cur.RatioToAnchor/ref.RatioToAnchor - 1
		row.Delta = &delta
		rows = append(rows, ChangeRow{
			BenchmarkRow: row,
			OldRatio:     ref.RatioToAnchor,
			NewRatio:     cur.RatioToAnchor,
		})
	}

	improvements := append([]ChangeRow(nil), rows...)
	sort.Slice(improvements, func(i, j int) bool {
		return *improvements[i].Delta < *improvements[j].Delta
	})
	slowdowns := append([]ChangeRow(nil), rows...)
	sort.Slice(slowdowns, func(i, j int) bool {
		return *slowdowns[i].Delta > *slowdowns[j].Delta
	})

	return takeSignificant(improvements, limit, true), takeSignificant(slowdowns, limit, false)
}

func takeSignificant(rows []ChangeRow, limit int, faster bool) []ChangeRow {
	out := make([]ChangeRow, 0, limit)
	for _, row := range rows {
		if row.Delta == nil {
			continue
		}
		if faster && *row.Delta >= -0.01 {
			continue
		}
		if !faster && *row.Delta <= 0.01 {
			continue
		}
		out = append(out, row)
		if len(out) == limit {
			return out
		}
	}
	return out
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func barWidth(ratio, maxRatio float64) float64 {
	if maxRatio <= 0 || ratio <= 0 {
		return 0
	}
	return math.Log1p(ratio) / math.Log1p(maxRatio) * 100
}

func renderPage(page PageData) ([]byte, error) {
	funcs := template.FuncMap{
		"date":       formatDate,
		"shortSHA":   shortSHA,
		"ns":         formatNS,
		"bytes":      formatBytes,
		"count":      formatCount,
		"ratio":      formatRatio,
		"pct":        formatPct,
		"deltaClass": deltaClass,
		"deltaText":  deltaText,
		"bar":        formatBar,
	}
	tmpl, err := template.New("page").Funcs(funcs).Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, page); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatDate(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func shortSHA(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func formatNS(value float64) string {
	return formatScaledUnit(value, "ns")
}

func formatBytes(value float64) string {
	return formatScaledUnit(value, "B")
}

func formatCount(value float64) string {
	number, prefix := splitBenchunitScale(benchunit.Scale(value, benchunit.Decimal))
	return trimScaledNumber(number) + prefix
}

func formatScaledUnit(value float64, unit string) string {
	tidiedValue, tidiedUnit := benchunit.Tidy(value, unit)
	number, prefix := splitBenchunitScale(benchunit.Scale(tidiedValue, benchunit.ClassOf(tidiedUnit)))
	return trimScaledNumber(number) + " " + displayUnit(prefix, tidiedUnit)
}

func splitBenchunitScale(value string) (string, string) {
	idx := len(value)
	for idx > 0 {
		r, size := utf8.DecodeLastRuneInString(value[:idx])
		if r == '.' || r == '-' || r == '+' || unicode.IsDigit(r) {
			break
		}
		idx -= size
	}
	return value[:idx], value[idx:]
}

func trimScaledNumber(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	if value == "-0" {
		return "0"
	}
	return value
}

func displayUnit(prefix, unit string) string {
	switch unit {
	case "sec":
		switch prefix {
		case "n":
			return "ns"
		case "µ":
			return "us"
		case "m":
			return "ms"
		case "":
			return "s"
		default:
			return prefix + "s"
		}
	case "B":
		return prefix + "B"
	default:
		return prefix + unit
	}
}

func formatRatio(value float64) string {
	if math.Abs(value) >= 1_000 {
		return formatCompact(value)
	}
	if value >= 10 {
		return fmt.Sprintf("%.1f", value)
	}
	return fmt.Sprintf("%.2f", value)
}

func formatCompact(value float64) string {
	abs := math.Abs(value)
	switch {
	case abs >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", value/1_000_000_000)
	case abs >= 1_000_000:
		return fmt.Sprintf("%.2fM", value/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%.1fk", value/1_000)
	default:
		return fmt.Sprintf("%.0f", value)
	}
}

func formatPct(value float64) string {
	return fmt.Sprintf("%+.1f%%", value*100)
}

func deltaClass(value *float64) string {
	if value == nil {
		return "muted"
	}
	if *value <= -0.05 {
		return "good"
	}
	if *value >= 0.05 {
		return "bad"
	}
	return "flat"
}

func deltaText(value *float64) string {
	if value == nil {
		return "new"
	}
	return formatPct(*value)
}

func formatBar(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} - let-go perf</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f7f7f4;
      --paper: #ffffff;
      --ink: #171717;
      --muted: #65645f;
      --line: #deddd6;
      --soft: #eeede7;
      --green: #167a48;
      --green-bg: #e4f3ea;
      --red: #aa2e2e;
      --red-bg: #f8e3e0;
      --amber: #8b5e12;
      --amber-bg: #f4ead2;
      --accent: #245c73;
      --shadow: 0 18px 50px rgba(23, 23, 23, 0.08);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--ink);
      font: 15px/1.5 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      letter-spacing: 0;
    }
    .wrap {
      width: min(1180px, calc(100% - 32px));
      margin: 0 auto;
    }
    header {
      padding: 34px 0 24px;
      border-bottom: 1px solid var(--line);
      background:
        linear-gradient(90deg, rgba(36, 92, 115, 0.12), transparent 42%),
        linear-gradient(180deg, #ffffff, var(--bg));
    }
    .topline {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 18px;
      margin-bottom: 28px;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 12px;
      min-width: 0;
      font-weight: 750;
      letter-spacing: 0;
    }
    .brand img {
      width: 42px;
      height: 42px;
      display: block;
    }
    .brand span {
      white-space: nowrap;
    }
    .links {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    .links a {
      color: var(--ink);
      text-decoration: none;
      border: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.7);
      border-radius: 7px;
      padding: 7px 10px;
      font-size: 13px;
      font-weight: 650;
    }
    h1 {
      margin: 0;
      font-size: clamp(42px, 7vw, 86px);
      line-height: 0.94;
      letter-spacing: 0;
      max-width: 820px;
    }
    .lede {
      margin: 18px 0 0;
      max-width: 760px;
      color: var(--muted);
      font-size: 18px;
    }
    .meta {
      margin-top: 20px;
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }
    .chip {
      display: inline-flex;
      align-items: center;
      min-height: 30px;
      padding: 5px 9px;
      border-radius: 7px;
      border: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.72);
      color: var(--muted);
      font-size: 13px;
      font-weight: 620;
    }
    main { padding: 28px 0 54px; }
    .cards {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
      margin-bottom: 30px;
    }
    .card {
      background: var(--paper);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      padding: 16px;
      min-height: 116px;
    }
    .label {
      margin: 0 0 8px;
      color: var(--muted);
      font-size: 12px;
      font-weight: 760;
      text-transform: uppercase;
      letter-spacing: 0.08em;
    }
    .value {
      margin: 0;
      font-size: 32px;
      line-height: 1;
      font-weight: 820;
      letter-spacing: 0;
    }
    .note {
      margin: 9px 0 0;
      color: var(--muted);
      font-size: 13px;
    }
    section {
      margin-top: 34px;
    }
    .section-head {
      display: flex;
      align-items: end;
      justify-content: space-between;
      gap: 20px;
      margin-bottom: 12px;
    }
    h2 {
      margin: 0;
      font-size: 24px;
      line-height: 1.15;
      letter-spacing: 0;
    }
    .section-head p {
      margin: 0;
      color: var(--muted);
      font-size: 13px;
    }
    .grid-2 {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
    }
    .chart-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
    }
    .chart {
      background: var(--paper);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      padding: 14px;
      min-width: 0;
    }
    .chart h3 {
      margin: 0;
      font-size: 16px;
      line-height: 1.2;
      letter-spacing: 0;
    }
    .chart p {
      margin: 5px 0 12px;
      color: var(--muted);
      font-size: 12px;
    }
    .chart svg {
      width: 100%;
      height: auto;
      display: block;
      overflow: visible;
    }
    .axis {
      stroke: var(--line);
      stroke-width: 1;
    }
    .axis-label {
      fill: var(--muted);
      font-size: 11px;
      font-variant-numeric: tabular-nums;
    }
    .chart-line {
      fill: none;
      stroke-width: 2.5;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
    .point {
      stroke: var(--paper);
      stroke-width: 2;
    }
    .legend {
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
      margin-top: 10px;
      color: var(--muted);
      font-size: 12px;
      font-weight: 650;
    }
    .legend span {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .swatch {
      width: 16px;
      height: 3px;
      border-radius: 999px;
      background: var(--series);
      display: inline-block;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      background: var(--paper);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      box-shadow: var(--shadow);
    }
    th, td {
      padding: 10px 12px;
      border-bottom: 1px solid var(--soft);
      text-align: left;
      vertical-align: middle;
    }
    th {
      color: var(--muted);
      font-size: 12px;
      font-weight: 780;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      background: #fbfbf9;
    }
    tr:last-child td { border-bottom: 0; }
    .bench {
      min-width: 260px;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 13px;
      line-height: 1.35;
      overflow-wrap: anywhere;
    }
    .pkg {
      display: block;
      color: var(--muted);
      font-family: ui-sans-serif, system-ui, sans-serif;
      font-size: 12px;
      margin-bottom: 2px;
    }
    .num {
      white-space: nowrap;
      font-variant-numeric: tabular-nums;
    }
    .delta {
      display: inline-flex;
      justify-content: center;
      min-width: 72px;
      padding: 4px 7px;
      border-radius: 6px;
      font-weight: 760;
      font-variant-numeric: tabular-nums;
    }
    .good { color: var(--green); background: var(--green-bg); }
    .bad { color: var(--red); background: var(--red-bg); }
    .flat { color: var(--amber); background: var(--amber-bg); }
    .muted { color: var(--muted); background: var(--soft); }
    .barcell {
      min-width: 170px;
    }
    .bartrack {
      height: 9px;
      border-radius: 999px;
      background: var(--soft);
      overflow: hidden;
    }
    .barfill {
      display: block;
      height: 100%;
      width: calc(var(--w) * 1%);
      min-width: 3px;
      background: linear-gradient(90deg, var(--accent), #6a9c7d);
      border-radius: inherit;
    }
    .empty {
      padding: 18px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--paper);
      color: var(--muted);
    }
    footer {
      padding: 22px 0 38px;
      color: var(--muted);
      border-top: 1px solid var(--line);
      font-size: 13px;
    }
    @media (max-width: 860px) {
      .cards, .grid-2, .chart-grid { grid-template-columns: 1fr; }
      .section-head { display: block; }
      .section-head p { margin-top: 6px; }
      table { display: block; overflow-x: auto; }
      .topline { align-items: flex-start; }
    }
    @media (max-width: 560px) {
      .wrap { width: min(100% - 22px, 1180px); }
      header { padding-top: 20px; }
      .topline { flex-direction: column; }
      .links { justify-content: flex-start; }
      h1 { font-size: 42px; }
      .lede { font-size: 16px; }
      th, td { padding: 9px; }
    }
  </style>
</head>
<body>
  <header>
    <div class="wrap">
      <div class="topline">
        <div class="brand">
          {{if .LogoDataURI}}<img alt="" src="{{.LogoDataURI}}">{{end}}
          <span>let-go perf</span>
        </div>
        <nav class="links" aria-label="Links">
          <a href="../">WASM repl</a>
          <a href="https://github.com/nooga/let-go">GitHub</a>
          <a href="https://github.com/nooga/let-go/blob/main/docs/perf/ratchet.md">Ratchet docs</a>
        </nav>
      </div>
      <h1>{{.Title}}</h1>
      <p class="lede">Committed benchmark ratchet data, rendered as a static page. Ratios are normalized to {{.Current.Anchor.Name}}, so lower is better and cross-machine drift is less noisy.</p>
      <div class="meta">
        <span class="chip">captured {{date .Current.CapturedAt}}</span>
        <span class="chip">sha {{shortSHA .Current.CapturedAtSHA}}</span>
        <span class="chip">{{.Current.Machine.CPUModel}}</span>
        <span class="chip">{{.Current.Machine.GoVersion}}</span>
      </div>
    </div>
  </header>

  <main class="wrap">
    <div class="cards" aria-label="Summary">
      <article class="card">
        <p class="label">Tracked benches</p>
        <p class="value">{{.Summary.BenchmarkCount}}</p>
        <p class="note">{{.Summary.PackageCount}} packages in the current baseline</p>
      </article>
      <article class="card">
        <p class="label">Zero allocs</p>
        <p class="value">{{.Summary.ZeroAllocs}}</p>
        <p class="note">benchmarks currently at 0 allocs/op</p>
      </article>
      <article class="card">
        <p class="label">Since {{.ReferenceName}}</p>
        <p class="value">{{pct .Summary.MedianDelta}}</p>
        <p class="note">median anchor-relative delta across {{.Summary.Common}} shared benches</p>
      </article>
      <article class="card">
        <p class="label">Movement</p>
        <p class="value">{{.Summary.Faster}} / {{.Summary.Slower}}</p>
        <p class="note">faster / slower by at least 5 percent</p>
      </article>
    </div>

    <section>
      <div class="section-head">
        <h2>Timeline</h2>
        <p>{{len .Timeline}} snapshot(s). CI snapshots graph real runs; seed points use committed historical/current JSON until the timeline fills in.</p>
      </div>
      {{if .Charts}}
      <div class="chart-grid">
        {{range .Charts}}
        <article class="chart">
          <h3>{{.Title}}</h3>
          <p>{{.Subtitle}}</p>
          <svg viewBox="0 0 520 210" role="img" aria-label="{{.Title}} trend chart">
            <line class="axis" x1="46" y1="22" x2="46" y2="176"></line>
            <line class="axis" x1="46" y1="176" x2="502" y2="176"></line>
            <text class="axis-label" x="4" y="29">{{.YMaxText}}</text>
            <text class="axis-label" x="4" y="176">{{.YMinText}}</text>
            <text class="axis-label" x="4" y="194">{{.Unit}}</text>
            <text class="axis-label" x="46" y="204">older</text>
            <text class="axis-label" x="461" y="204">newer</text>
            {{range .Series}}
            {{$color := .Color}}
            <path class="chart-line" stroke="{{$color}}" d="{{.Path}}"></path>
            {{range .Points}}
            <circle class="point" fill="{{$color}}" cx="{{printf "%.2f" .X}}" cy="{{printf "%.2f" .Y}}" r="4">
              <title>{{.Date}} @ {{.SHA}}: {{.Text}}</title>
            </circle>
            {{end}}
            {{end}}
          </svg>
          <div class="legend">
            {{range .Series}}<span><i class="swatch" style="--series: {{.Color}}"></i>{{.Label}}</span>{{end}}
          </div>
        </article>
        {{end}}
      </div>
      {{else}}
      <div class="empty">No timeline-capable benchmarks found yet.</div>
      {{end}}
    </section>

    <section>
      <div class="section-head">
        <h2>Largest changes</h2>
        <p>Current ratchet compared with {{.ReferenceName}}.</p>
      </div>
      <div class="grid-2">
        {{if .TopImprovements}}
        <table>
          <thead><tr><th>Improved</th><th>Delta</th><th>Now</th></tr></thead>
          <tbody>
            {{range .TopImprovements}}
            <tr>
              <td class="bench"><span class="pkg">{{.Package}}</span>{{.Name}}</td>
              <td><span class="delta good">{{deltaText .Delta}}</span></td>
              <td class="num">{{ratio .NewRatio}} anchors</td>
            </tr>
            {{end}}
          </tbody>
        </table>
        {{else}}<div class="empty">No historical improvements found.</div>{{end}}

        {{if .TopSlowdowns}}
        <table>
          <thead><tr><th>Slower</th><th>Delta</th><th>Now</th></tr></thead>
          <tbody>
            {{range .TopSlowdowns}}
            <tr>
              <td class="bench"><span class="pkg">{{.Package}}</span>{{.Name}}</td>
              <td><span class="delta bad">{{deltaText .Delta}}</span></td>
              <td class="num">{{ratio .NewRatio}} anchors</td>
            </tr>
            {{end}}
          </tbody>
        </table>
        {{else}}<div class="empty">No historical slowdowns found.</div>{{end}}
      </div>
    </section>

    <section>
      <div class="section-head">
        <h2>Recently tightened</h2>
        <p>Most recent per-benchmark bars in the ratchet.</p>
      </div>
      <table>
        <thead><tr><th>Benchmark</th><th>Bar set</th><th>Wall</th><th>Allocs</th></tr></thead>
        <tbody>
          {{range .RecentlyTightened}}
          <tr>
            <td class="bench"><span class="pkg">{{.Package}}</span>{{.Name}}</td>
            <td class="num">{{date .BestSinceAt}} @ {{shortSHA .BestSinceSHA}}</td>
            <td class="num">{{ns .NSPerOp}}</td>
            <td class="num">{{count .AllocsPerOp}}</td>
          </tr>
          {{end}}
        </tbody>
      </table>
    </section>

    <section>
      <div class="section-head">
        <h2>Current baseline</h2>
        <p>Sorted by package and benchmark. Lower anchor ratio is faster.</p>
      </div>
      <table>
        <thead>
          <tr>
            <th>Benchmark</th>
            <th>Ratio</th>
            <th>Wall</th>
            <th>Alloc</th>
            <th>Bytes</th>
            <th>Delta</th>
            <th>Scale</th>
          </tr>
        </thead>
        <tbody>
          {{range .Rows}}
          <tr>
            <td class="bench"><span class="pkg">{{.Package}}</span>{{.Name}}</td>
            <td class="num">{{ratio .Ratio}}</td>
            <td class="num">{{ns .NSPerOp}}</td>
            <td class="num">{{count .AllocsPerOp}}</td>
            <td class="num">{{bytes .BytesPerOp}}</td>
            <td><span class="delta {{deltaClass .Delta}}">{{deltaText .Delta}}</span></td>
            <td class="barcell"><div class="bartrack"><span class="barfill" style="--w: {{bar .BarWidth}}"></span></div></td>
          </tr>
          {{end}}
        </tbody>
      </table>
    </section>
  </main>

  <footer>
    <div class="wrap">Source data: docs/perf/baseline.json, docs/perf/historical/*.json, and docs/perf/timeline/*.json. Page generation does not run benchmarks.</div>
  </footer>
</body>
</html>
`
