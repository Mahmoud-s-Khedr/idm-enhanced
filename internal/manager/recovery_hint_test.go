package manager

import (
	"strings"
	"testing"
)

func TestRecoveryHintKeepsUnsafeResumeAndSecretsOutOfAdvice(t *testing.T) {
	if hint := RecoveryHint(Job{State: "needs_attention", Error: "transfer"}); !strings.Contains(hint, "Restart") || strings.Contains(hint, "Resume / retry") {
		t.Fatalf("unsafe protocol advised resume: %q", hint)
	}
	for _, test := range []struct{ category, next string }{
		{"", ""},
		{"remote_identity", "Restart"},
		{"checksum", "checksum"},
		{"local_identity", "new destination"},
		{"publication_recovery", "Preserve existing files"},
		{"publication", "destination"},
		{"discarded", "Restart"},
		{"checkpoint", "disk space"},
		{"verification", "disk space"},
		{"sync", "disk space"},
		{"interrupted", "Resume / retry"},
		{"transfer", "Browser session credentials are unsupported"},
		{"unknown-secret", "source access"},
	} {
		t.Run(test.category, func(t *testing.T) {
			j := Job{Error: test.category, Source: "https://example.invalid/?secret=private", Partial: "private-partial"}
			hint := RecoveryHint(j)
			if (test.category == "" && hint != "") || !strings.Contains(hint, test.next) {
				t.Fatalf("unhelpful recovery advice: %q", hint)
			}
			for _, secret := range []string{"private", "unknown-secret", "https://"} {
				if strings.Contains(hint, secret) {
					t.Fatalf("recovery hint exposed metadata: %q", hint)
				}
			}
		})
	}
}
