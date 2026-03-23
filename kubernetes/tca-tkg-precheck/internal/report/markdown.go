package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func RenderMarkdown(r Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", r.Title)
	fmt.Fprintf(&b, "- Generated: %s\n", r.Generated.Format(time.RFC3339))
	if r.Context != "" {
		fmt.Fprintf(&b, "- Context: %s\n", r.Context)
	}
	b.WriteString("\n## Table of contents\n\n")
	for _, res := range r.Results {
		fmt.Fprintf(&b, "- [%s %s](#%s)\n", res.ID, res.Category, anchor(res.ID+" "+res.Category))
	}

	b.WriteString("\n## Summary\n\n")
	b.WriteString("| ID | Category | Status |\n")
	b.WriteString("|---:|----------|--------|\n")
	for _, res := range r.Results {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", res.ID, res.Category, res.Status)
	}

	for _, res := range r.Results {
		fmt.Fprintf(&b, "\n## %s %s\n\n", res.ID, res.Category)
		fmt.Fprintf(&b, "**Status:** %s\n\n", res.Status)
		if res.Summary != "" {
			fmt.Fprintf(&b, "%s\n\n", res.Summary)
		}
		if len(res.Details) > 0 {
			b.WriteString("**Details**\n\n")
			for _, d := range res.Details {
				fmt.Fprintf(&b, "- %s\n", d)
			}
			b.WriteString("\n")
		}
		if len(res.NextSteps) > 0 {
			b.WriteString("**Next steps**\n\n")
			for _, ns := range res.NextSteps {
				fmt.Fprintf(&b, "- `%s`\n", ns)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n## Next steps (overall)\n\n")
	b.WriteString("- Re-run after fixing FAIL/WARN items: `precheck --output-md report.md`\n")

	return b.String()
}

func anchor(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		default:
			// drop
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	out = strings.ReplaceAll(out, "--", "-")
	return out
}

func UniqueSorted(ss []string) []string {
	m := map[string]struct{}{}
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		m[s] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
