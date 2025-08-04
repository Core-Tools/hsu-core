//go:build test

package processcontrolimpl

import (
	"testing"

	"github.com/core-tools/hsu-core/pkg/workers/processcontrol"
)

// ===== PERFORMANCE BENCHMARKS =====

// BenchmarkProcessControl_StateReads measures state reading performance
func BenchmarkProcessControl_StateReads(b *testing.B) {
	logger := &SimpleLogger{}

	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-worker", logger)
	impl := pc.(*processControl)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		impl.GetState()
	}
}

// BenchmarkProcessControl_ConcurrentReads measures concurrent state reading performance
func BenchmarkProcessControl_ConcurrentReads(b *testing.B) {
	logger := &SimpleLogger{}

	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-worker", logger)
	impl := pc.(*processControl)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			impl.GetState()
		}
	})
}
