package loadtest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/loadtest"
)

func TestReportTwoProcessPoolLabel(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	rep := loadtest.Report{
		Scenario: "create",
		Machine:  loadtest.Machine{OS: "linux", Arch: "amd64", CPUs: 8, MemTotalKB: 16 * 1024 * 1024},
		Env: loadtest.EnvLabels{
			DBPoolPerProcess:     25,
			WorkerPoolPerProcess: 4,
			BankDelayMS:          200,
			BankMix:              "pinned 100/0/0/0",
			APIProcesses:         2,
		},
		Attack: &loadtest.AttackStats{
			Requests:    100,
			Rate:        15,
			StatusCodes: map[string]int{"201": 100},
			P50:         10 * time.Millisecond,
			P95:         20 * time.Millisecond,
			P99:         30 * time.Millisecond,
			Success:     1,
		},
		AuthLatency:  loadtest.LatencyStats{N: 100, P50: 205 * time.Millisecond},
		StatusCounts: map[string]int{"AUTHORIZED": 100},
	}
	if err := rep.Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"db_pool=25×2", "worker_pool=4×2", "bank_delay_ms=200", "checks: ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}
