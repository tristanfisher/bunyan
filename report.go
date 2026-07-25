package bunyan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"
)

type ZoneReport struct{}

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
}

func (cr ChainReport) FlameText() {

}

var chainTemplateFunc = template.FuncMap{
	"increment": func(i int) int {
		i++
		return i
	},
}

// chainReportTemplate gives us a string-based reporting.  the increment function is used for 1-based indexing.
const chainReportTemplate = `ID: {{ .ID }}
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

func processCategoryTable(table map[string]entries) {
	fmt.Println("## processCategoryTable")
	for categoryName, entries := range table {
		fmt.Println("reporting: ", categoryName)
		fmt.Println("reporting: ", entries)
	}
	fmt.Println("## /processCategoryTable")

	// process all start/end out of table to make flamegraph
}
