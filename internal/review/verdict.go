package review

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReviewVerdict is the structured output from a gate review agent.
type ReviewVerdict struct {
	BeadID             string      `json:"bead_id"`
	Verdict            string      `json:"verdict"`
	ScopeItems         []ScopeItem `json:"scope_items"`
	GateCheckConsistent bool       `json:"gate_check_consistent"`
	Summary            string      `json:"summary"`
}

// ScopeItem is one verifiable claim from the bead's done-condition.
type ScopeItem struct {
	Item       string `json:"item"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Valid verdict values.
const (
	VerdictPass    = "PASS"
	VerdictPartial = "PARTIAL"
	VerdictFail    = "FAIL"
	VerdictUnclear = "UNCLEAR"
)

// ParseVerdict reads and validates a review verdict from JSON bytes.
func ParseVerdict(data []byte) (*ReviewVerdict, error) {
	var v ReviewVerdict
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("invalid verdict JSON: %w", err)
	}

	// Validate verdict value
	switch v.Verdict {
	case VerdictPass, VerdictPartial, VerdictFail, VerdictUnclear:
		// valid
	default:
		return nil, fmt.Errorf("invalid verdict value %q: must be PASS, PARTIAL, FAIL, or UNCLEAR", v.Verdict)
	}

	// Validate scope items
	for i, item := range v.ScopeItems {
		if item.Item == "" {
			return nil, fmt.Errorf("scope_items[%d]: missing item field", i)
		}
		switch item.Status {
		case "PASS", "FAIL", "UNCLEAR":
			// valid
		default:
			return nil, fmt.Errorf("scope_items[%d]: invalid status %q", i, item.Status)
		}
	}

	return &v, nil
}

// ExitCodeForVerdict returns the appropriate exit code for a verdict.
// 0=PASS, 1=PARTIAL/FAIL, 2=UNCLEAR.
func ExitCodeForVerdict(v *ReviewVerdict) int {
	switch v.Verdict {
	case VerdictPass:
		return 0
	case VerdictUnclear:
		return 2
	default: // PARTIAL, FAIL
		return 1
	}
}

// FormatPrettyVerdict produces colorized terminal output for a review verdict.
func FormatPrettyVerdict(v *ReviewVerdict) string {
	var sb strings.Builder

	// Header with verdict
	verdictColor := verdictColorCode(v.Verdict)
	sb.WriteString(fmt.Sprintf("\n%sREVIEW VERDICT: %s\033[0m\n\n", verdictColor, v.Verdict))

	// Scope items
	for _, item := range v.ScopeItems {
		icon := statusIcon(item.Status)
		sb.WriteString(fmt.Sprintf("  %s %s\n", icon, item.Item))
		if item.Evidence != "" {
			sb.WriteString(fmt.Sprintf("    Evidence: %s\n", item.Evidence))
		}
		if item.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("    Suggestion: %s\n", item.Suggestion))
		}
		sb.WriteString("\n")
	}

	// Summary
	if v.Summary != "" {
		sb.WriteString(fmt.Sprintf("%s\n", v.Summary))
	}

	// Score line
	passed := 0
	total := len(v.ScopeItems)
	for _, item := range v.ScopeItems {
		if item.Status == "PASS" {
			passed++
		}
	}
	if total > 0 {
		sb.WriteString(fmt.Sprintf("\n%d of %d scope items met.\n", passed, total))
	}

	return sb.String()
}

// verdictColorCode returns the ANSI color code for a verdict.
func verdictColorCode(verdict string) string {
	switch verdict {
	case VerdictPass:
		return "\033[32m" // green
	case VerdictPartial:
		return "\033[33m" // yellow
	case VerdictFail:
		return "\033[31m" // red
	case VerdictUnclear:
		return "\033[35m" // magenta
	default:
		return ""
	}
}

// statusIcon returns a colored icon for a scope item status.
func statusIcon(status string) string {
	switch status {
	case "PASS":
		return "\033[32m\u2713\033[0m" // green checkmark
	case "FAIL":
		return "\033[31m\u2717\033[0m" // red X
	case "UNCLEAR":
		return "\033[35m?\033[0m" // magenta question mark
	default:
		return "-"
	}
}
