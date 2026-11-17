package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// This is an applicability decision, not a claim that picomatch is patched.
// Scope, evidence, reviewer and expiry: docs/security/GHSA-fc5q-39xc-vjpf.md.
const picomatchAdvisory = "GHSA-fc5q-39xc-vjpf"
const picomatchReviewExpires = "2026-12-01"

type auditFinding struct {
	Version string   `json:"version"`
	Paths   []string `json:"paths"`
	Dev     *bool    `json:"dev"`
}
type auditAdvisory struct {
	Package  string         `json:"module_name"`
	ID       string         `json:"github_advisory_id"`
	Severity string         `json:"severity"`
	Findings []auditFinding `json:"findings"`
}
type auditCounts struct {
	Info     int  `json:"info"`
	Low      int  `json:"low"`
	Moderate int  `json:"moderate"`
	High     *int `json:"high"`
	Critical *int `json:"critical"`
}
type auditReport struct {
	Advisories map[string]auditAdvisory `json:"advisories"`
	Metadata   *struct {
		Counts *auditCounts `json:"vulnerabilities"`
	} `json:"metadata"`
	Error json.RawMessage `json:"error"`
}

func javascriptAudit() error {
	fmt.Println("+", pnpmExecutable(), "audit --audit-level=high --json")
	data, scanErr := output(pnpmExecutable(), "audit", "--audit-level=high", "--json")
	if scanErr != nil {
		var exit *exec.ExitError
		if !errors.As(scanErr, &exit) || exit.ExitCode() != 1 {
			return fmt.Errorf("pnpm audit execution failed: %w", scanErr)
		}
	}
	summary, err := assessAudit(data, time.Now().UTC(), scanErr != nil)
	if err != nil {
		return err
	}
	fmt.Println(summary)
	return nil
}

func assessAudit(data []byte, now time.Time, scanFailed bool) (string, error) {
	var report auditReport
	if err := json.Unmarshal(data, &report); err != nil {
		return "", errors.New("pnpm audit returned invalid JSON")
	}
	if report.Advisories == nil || report.Metadata == nil || report.Metadata.Counts == nil || report.Metadata.Counts.High == nil || report.Metadata.Counts.Critical == nil || (len(report.Error) > 0 && string(report.Error) != "null") {
		return "", errors.New("pnpm audit returned an incomplete or failed report")
	}
	counts := *report.Metadata.Counts
	if counts.Info < 0 || counts.Low < 0 || counts.Moderate < 0 || *counts.High < 0 || *counts.Critical < 0 {
		return "", errors.New("pnpm audit returned invalid vulnerability counts")
	}
	seen := map[string]int{}
	var blocked, reviewed []string
	expiry, err := time.Parse(time.DateOnly, picomatchReviewExpires)
	if err != nil {
		return "", err
	}
	for _, advisory := range report.Advisories {
		switch advisory.Severity {
		case "info", "low", "moderate":
			seen[advisory.Severity]++
			continue
		case "high", "critical":
			seen[advisory.Severity]++
		default:
			return "", errors.New("pnpm audit returned an unknown severity")
		}
		applicable := true
		if advisory.ID == picomatchAdvisory && advisory.Package == "picomatch" && advisory.Severity == "high" && now.Before(expiry) && len(advisory.Findings) > 0 {
			applicable = false
			for _, finding := range advisory.Findings {
				if finding.Version != "4.0.7" || finding.Dev == nil || !*finding.Dev || len(finding.Paths) == 0 {
					applicable = true
					break
				}
				for _, path := range finding.Paths {
					if strings.TrimSpace(path) == "" {
						applicable = true
						break
					}
				}
				if applicable {
					break
				}
			}
		}
		label := advisory.ID + " (" + advisory.Package + ", " + advisory.Severity + ")"
		if applicable {
			blocked = append(blocked, label)
		} else {
			reviewed = append(reviewed, label)
		}
	}
	if seen["info"] != counts.Info || seen["low"] != counts.Low || seen["moderate"] != counts.Moderate || seen["high"] != *counts.High || seen["critical"] != *counts.Critical {
		return "", errors.New("pnpm audit vulnerability counts disagree with advisory details")
	}
	// JSON mode reports all severities and exits 1 for any finding, even
	// below --audit-level. Require complete details, then apply our threshold.
	if scanFailed && len(report.Advisories) == 0 {
		return "", errors.New("pnpm audit failed without advisory details")
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		return "", fmt.Errorf("applicable high/critical dependency findings: %s", strings.Join(blocked, ", "))
	}
	if len(reviewed) > 0 {
		return fmt.Sprintf("REVIEWED development-only applicability exception until %s UTC: %s; picomatch@4.0.7 is unpatched. See docs/security/GHSA-fc5q-39xc-vjpf.md", picomatchReviewExpires, strings.Join(reviewed, ", ")), nil
	}
	return "No high/critical JavaScript dependency findings.", nil
}
