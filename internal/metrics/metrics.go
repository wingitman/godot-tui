package metrics

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

type Snapshot struct {
	At          time.Time
	CPUPercent  float64
	MemoryBytes uint64
	Available   bool
	Error       string
}

func Sample(pid int32) Snapshot {
	s := Snapshot{At: time.Now()}
	p, err := process.NewProcess(pid)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	cpu, cpuErr := p.CPUPercent()
	mem, memErr := p.MemoryInfo()
	if cpuErr != nil && memErr != nil {
		s.Error = fmt.Sprintf("cpu: %v; memory: %v", cpuErr, memErr)
		return s
	}
	s.CPUPercent = cpu
	if mem != nil {
		s.MemoryBytes = mem.RSS
	}
	s.Available = true
	return s
}
