package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Status string

const (
	StatusPass Status = "PASS"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
)

type CheckResult struct {
	ID       string
	Category string
	Status   Status

	Summary string
	Details []string

	NextSteps []string
}

type Report struct {
	Title     string
	Generated time.Time
	Context   string

	Results []CheckResult
}

func (r Report) HasFail() bool {
	for _, res := range r.Results {
		if res.Status == StatusFail {
			return true
		}
	}
	return false
}

func WorstStatus(a, b Status) Status {
	if a == StatusFail || b == StatusFail {
		return StatusFail
	}
	if a == StatusWarn || b == StatusWarn {
		return StatusWarn
	}
	return StatusPass
}

type PrinterOptions struct {
	Color bool
}

type Printer struct {
	w    io.Writer
	opts PrinterOptions
}

func NewPrinter(w io.Writer, opts PrinterOptions) *Printer {
	return &Printer{w: w, opts: opts}
}

func (p *Printer) Print(r Report) {
	fmt.Fprintf(p.w, "%s\n", r.Title)
	fmt.Fprintf(p.w, "Generated: %s\n", r.Generated.Format(time.RFC3339))
	if r.Context != "" {
		fmt.Fprintf(p.w, "Context:   %s\n", r.Context)
	}
	fmt.Fprintln(p.w)

	p.printSummaryTable(r)
	fmt.Fprintln(p.w)

	for _, res := range r.Results {
		fmt.Fprintf(p.w, "[%s] %s: %s\n", res.ID, res.Category, p.colorStatus(res.Status))
		if res.Summary != "" {
			fmt.Fprintf(p.w, "  %s\n", res.Summary)
		}
		for _, d := range res.Details {
			fmt.Fprintf(p.w, "  - %s\n", d)
		}
		if len(res.NextSteps) > 0 {
			fmt.Fprintln(p.w, "  Next steps:")
			for _, ns := range res.NextSteps {
				fmt.Fprintf(p.w, "  - %s\n", ns)
			}
		}
		fmt.Fprintln(p.w)
	}

	fmt.Fprintln(p.w, "Next steps (overall):")
	fmt.Fprintln(p.w, "  - Re-run after fixing FAIL/WARN items: precheck --output-md report.md")
}

func (p *Printer) printSummaryTable(r Report) {
	maxCat := 0
	for _, res := range r.Results {
		if l := len(res.Category); l > maxCat {
			maxCat = l
		}
	}
	if maxCat < 10 {
		maxCat = 10
	}

	fmt.Fprintln(p.w, "Summary:")
	for _, res := range r.Results {
		fmt.Fprintf(p.w, "  %-3s %-*s %s\n", res.ID, maxCat, res.Category, p.colorStatus(res.Status))
	}
}

func (p *Printer) colorStatus(s Status) string {
	if !p.opts.Color {
		return string(s)
	}
	switch s {
	case StatusPass:
		return "\x1b[32m" + string(s) + "\x1b[0m"
	case StatusWarn:
		return "\x1b[33m" + string(s) + "\x1b[0m"
	case StatusFail:
		return "\x1b[31m" + string(s) + "\x1b[0m"
	default:
		return string(s)
	}
}

func StdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func SanitizeOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
