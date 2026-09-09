package bunyan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"
)

type SpanReport struct {
	SpanID       SpanID
	ParentSpanID SpanID
	Category     string
	Start        time.Time
	End          time.Time
	Comment      []string
}

// spanAnalysis is used for reports and statistical analysis.
// fields are exported for templating.
type spanAnalysis struct {
	Span              *span
	Duration          time.Duration
	FlameCalculations struct {
		Offset time.Duration
		Width  time.Duration
		Depth  int
	}
}

// ChainReport is a serializable set of fields
// from a given chain.
type ChainReport struct {
	ID            ChainID
	TotalDuration time.Duration
	Start         time.Time
	End           time.Time
	Categories    CategoriesReport
	CategoryNames []string
	LongestSpan   spanAnalysis
	SpanTimings   []spanAnalysis
	Warnings      []string
	FlameText     string
}

var chainTemplateFunc = template.FuncMap{
	"increment": func(i int) int {
		i++
		return i
	},
}

// chainReportTemplate gives us a string-based reporting.  the increment function is used for 1-based indexing.
const chainReportTemplate = `=== CHAIN REPORT: {{ .ID }} ===
Warnings:
{{- range .Warnings }}
- {{ . }}
{{- end }}

Categories: {{ .Categories }}
Category names: 
{{- range .CategoryNames }}
- {{ . }} 
{{- end }}
Category comment: {{ .Categories.Comment }}

# Timing
TotalDuration: {{ .TotalDuration }}
LongestSpan: {{ .LongestSpan }}
SpanTimings: 
{{- range $i, $span := .SpanTimings }}
 {{ increment $i }}. Span: {{ $span.Span }} | Duration: {{ $span.Duration }}
{{- else }}
  No spans recorded.
{{- end }}
`

func (cr ChainReport) String() string {
	t, err := template.New("chainReport").Funcs(chainTemplateFunc).Parse(chainReportTemplate)
	if err != nil {
		return fmt.Errorf("<error templating report: %w>", err).Error()
	}
	stringBuf := bytes.Buffer{}
	err = t.Execute(&stringBuf, cr)
	if err != nil {
		return fmt.Errorf("<error executing template for report: %w>", err).Error()
	}
	return stringBuf.String()
}

func (cr ChainReport) ToJSON() (string, error) {
	b, err := json.Marshal(cr)
	return string(b), err
}

type CategoriesReport struct {
	// expected to match category.table
	CategoryTable map[string]entries
	Comment       []string
}

type ZoneReport struct {
	ID            ZoneID
	TotalDuration time.Duration
	Chains        []ChainReport
	Warnings      []string
}

var zoneTemplateFunc = template.FuncMap{
	"increment": func(i int) int {
		i++
		return i
	},
}

const zoneReportTemplate = `=== ZONE REPORT: {{ .ID }} ===
{{- if .Warnings }}
Warnings:
{{- range .Warnings }}
- {{ . }}
{{- end }}
{{- end }}
Total Duration: {{ .TotalDuration }}
Chains ({{ len .Chains }}):
{{- range $i, $chain := .Chains }}

--- Chain [{{ increment $i }}]: {{ $chain.ID }} ---
Total Duration: {{ $chain.TotalDuration }}
Categories ({{ len $chain.CategoryNames }}): {{ range $chain.CategoryNames }}[{{ . }}] {{ end }}
Span Timings:
{{- range $j, $span := $chain.SpanTimings }}
  {{ increment $j }}. Span: {{ $span.Span }} | Duration: {{ $span.Duration }}
{{- else }}
  No spans recorded.
{{- end }}
{{- else }}
  No chains recorded.
{{- end }}
===================================
`

func (zr ZoneReport) String() string {
	t, err := template.New("zoneReport").Funcs(zoneTemplateFunc).Parse(zoneReportTemplate)
	if err != nil {
		return fmt.Errorf("<error templating report: %w>", err).Error()
	}

	stringBuf := bytes.Buffer{}
	err = t.Execute(&stringBuf, zr)
	if err != nil {
		return fmt.Errorf("<error executing template for report: %w>", err).Error()
	}
	return stringBuf.String()
}

func (zr ZoneReport) ToJSON() (string, error) {
	b, err := json.Marshal(zr)
	return string(b), err
}
