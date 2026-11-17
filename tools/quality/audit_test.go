package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const reviewedAuditFixture = `{"advisories":{"1247710":{"module_name":"picomatch","github_advisory_id":"GHSA-fc5q-39xc-vjpf","severity":"high","findings":[{"version":"4.0.7","dev":true,"paths":[".>vite>picomatch"]}]}},"metadata":{"vulnerabilities":{"info":0,"low":0,"moderate":0,"high":1,"critical":0}}}`

func TestAuditApplicability(t *testing.T) {
	before := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		change  func(*auditReport)
		now     time.Time
		allowed bool
	}{
		{name: "reviewed dev tooling", allowed: true},
		{name: "production", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			prod := false
			a.Findings[0].Dev = &prod
			r.Advisories["1247710"] = a
		}},
		{name: "missing dev evidence", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Findings[0].Dev = nil
			r.Advisories["1247710"] = a
		}},
		{name: "older version", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Findings[0].Version = "4.0.6"
			r.Advisories["1247710"] = a
		}},
		{name: "new affected version", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Findings[0].Version = "4.0.8"
			r.Advisories["1247710"] = a
		}},
		{name: "another package", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Package = "another-package"
			r.Advisories["1247710"] = a
		}},
		{name: "another advisory", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.ID = "GHSA-other-finding"
			r.Advisories["1247710"] = a
		}},
		{name: "critical severity", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Severity = "critical"
			r.Advisories["1247710"] = a
			*r.Metadata.Counts.High = 0
			*r.Metadata.Counts.Critical = 1
		}},
		{name: "mixed production and dev", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			prod := false
			a.Findings = append(a.Findings, auditFinding{Version: "4.0.7", Dev: &prod, Paths: []string{"frontend>picomatch"}})
			r.Advisories["1247710"] = a
		}},
		{name: "another high beside reviewed finding", change: func(r *auditReport) {
			r.Advisories["other"] = auditAdvisory{Package: "other", ID: "GHSA-other-finding", Severity: "high"}
			(*r.Metadata.Counts.High)++
		}},
		{name: "missing findings", change: func(r *auditReport) { a := r.Advisories["1247710"]; a.Findings = nil; r.Advisories["1247710"] = a }},
		{name: "empty path", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Findings[0].Paths = []string{" "}
			r.Advisories["1247710"] = a
		}},
		{name: "missing paths", change: func(r *auditReport) {
			a := r.Advisories["1247710"]
			a.Findings[0].Paths = nil
			r.Advisories["1247710"] = a
		}},
		{name: "expired exactly at deadline", now: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{name: "expired after deadline", now: time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var report auditReport
			if err := json.Unmarshal([]byte(reviewedAuditFixture), &report); err != nil {
				t.Fatal(err)
			}
			if tc.change != nil {
				tc.change(&report)
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			now := tc.now
			if now.IsZero() {
				now = before
			}
			summary, err := assessAudit(data, now, true)
			if (err == nil) != tc.allowed {
				t.Fatalf("summary=%q error=%v", summary, err)
			}
			if tc.allowed && (!strings.Contains(summary, "REVIEWED") || !strings.Contains(summary, "unpatched") || !strings.Contains(summary, picomatchReviewExpires)) {
				t.Fatal("exception was not disclosed", summary)
			}
		})
	}
}
func TestAuditFailsClosed(t *testing.T) {
	for _, data := range []string{
		"not JSON", `{}`, `{"advisories":{},"metadata":{}}`,
		`{"advisories":{},"metadata":{"vulnerabilities":{"high":1}}}`,
		`{"advisories":{},"metadata":{"vulnerabilities":{"high":-1,"critical":0}}}`,
		`{"advisories":{},"metadata":{"vulnerabilities":{"high":0}}}`,
		`{"advisories":{},"metadata":{"vulnerabilities":{}},"error":{"message":"registry unavailable"}}`,
		`{"advisories":{"x":{"severity":"unknown"}},"metadata":{"vulnerabilities":{}}}`,
		`{"advisories":null,"metadata":{"vulnerabilities":{}}}`,
	} {
		if _, err := assessAudit([]byte(data), time.Now(), false); err == nil {
			t.Fatal("accepted malformed/failed report", data)
		}
	}
	const clean = `{"advisories":{},"metadata":{"vulnerabilities":{"info":0,"low":0,"moderate":0,"high":0,"critical":0}}}`
	if _, err := assessAudit([]byte(clean), time.Now(), true); err == nil {
		t.Fatal("accepted scanner failure without finding details")
	}
	if summary, err := assessAudit([]byte(clean), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), false); err != nil || strings.Contains(summary, "exception") {
		t.Fatal(summary, err)
	}
	const lower = `{"advisories":{"x":{"module_name":"other","github_advisory_id":"GHSA-other","severity":"moderate"}},"metadata":{"vulnerabilities":{"moderate":1,"high":0,"critical":0}}}`
	for _, scanFailed := range []bool{false, true} {
		if _, err := assessAudit([]byte(lower), time.Now(), scanFailed); err != nil {
			t.Fatal("changed high/critical threshold", err)
		}
	}
	const missingLowerDetails = `{"advisories":{},"metadata":{"vulnerabilities":{"moderate":1,"high":0,"critical":0}}}`
	if _, err := assessAudit([]byte(missingLowerDetails), time.Now(), true); err == nil {
		t.Fatal("accepted missing lower-severity details")
	}
	const lowerWithScannerError = `{"advisories":{"x":{"module_name":"other","github_advisory_id":"GHSA-other","severity":"moderate"}},"metadata":{"vulnerabilities":{"moderate":1,"high":0,"critical":0}},"error":{"message":"registry failed"}}`
	if _, err := assessAudit([]byte(lowerWithScannerError), time.Now(), true); err == nil {
		t.Fatal("accepted scanner failure beside lower-severity finding")
	}
}
