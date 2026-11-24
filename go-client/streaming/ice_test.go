package streaming

import (
	"strings"
	"testing"
)

func TestNormalizeICE(t *testing.T) {
	in := []ICECandidate{{Candidate: "candidate:1 1 UDP 100 192.0.2.1 1234 typ host"}, {Candidate: "a=candidate:2 1 UDP 10 2001:0:4136:e378:8000:63bf:3fff:fdd2 4567 typ host"}, {Candidate: "a=end-of-candidates"}}
	got, err := NormalizeICE(in, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(got[0].Candidate, "2001:0:4136:e378:8000:63bf:3fff:fdd2") || !strings.Contains(got[0].Candidate, "2130706431") {
		t.Fatal(got[0])
	}
	if !strings.Contains(got[2].Candidate, "192.0.2.45 9002") || !strings.Contains(got[3].Candidate, "192.0.2.45 40000") {
		t.Fatal(got)
	}
	if got[4].Candidate != "a=end-of-candidates" {
		t.Fatal(got[4])
	}
}
func TestNormalizeICERejectsMalformed(t *testing.T) {
	for _, candidate := range []string{"bad", "candidate:1 1 UDP 1 no-ip 80 typ host", "candidate:1 1 UDP 1 1.2.3.4 0 typ host"} {
		if _, err := NormalizeICE([]ICECandidate{{Candidate: candidate}}, false); err == nil {
			t.Fatalf("accepted %q", candidate)
		}
	}
}
