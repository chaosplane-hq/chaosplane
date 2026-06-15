package ebpf

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

type Manager struct {
	logger   *slog.Logger
	mu       sync.Mutex
	programs map[string]*loadedProgram
}

type loadedProgram struct {
	prog *ebpf.Program
	link link.Link
	spec *ebpf.ProgramSpec
}

func NewManager(logger *slog.Logger) *Manager {
	if err := rlimit.RemoveMemlock(); err != nil {
		logger.Warn("failed to remove memlock rlimit, eBPF may not work", "error", err)
	}
	return &Manager{
		logger:   logger,
		programs: make(map[string]*loadedProgram),
	}
}

func (m *Manager) LoadTCDelay(executionID string, ifIndex int, delayUS uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	spec := &ebpf.ProgramSpec{
		Name:         "tc_delay_" + executionID[:8],
		Type:         ebpf.SchedCLS,
		License:      "GPL",
		Instructions: delayBPFInstructions(delayUS),
	}

	prog, err := ebpf.NewProgram(spec)
	if err != nil {
		return fmt.Errorf("load tc delay program: %w", err)
	}

	l, err := link.AttachTCX(link.TCXOptions{
		Interface: ifIndex,
		Program:   prog,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err != nil {
		prog.Close()
		return fmt.Errorf("attach tc delay program: %w", err)
	}

	m.programs[executionID] = &loadedProgram{prog: prog, link: l, spec: spec}
	m.logger.Info("eBPF tc delay loaded", "executionID", executionID, "ifIndex", ifIndex, "delayUS", delayUS)
	return nil
}

func (m *Manager) LoadTCDrop(executionID string, ifIndex int, dropPercent uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	spec := &ebpf.ProgramSpec{
		Name:         "tc_drop_" + executionID[:8],
		Type:         ebpf.SchedCLS,
		License:      "GPL",
		Instructions: dropBPFInstructions(dropPercent),
	}

	prog, err := ebpf.NewProgram(spec)
	if err != nil {
		return fmt.Errorf("load tc drop program: %w", err)
	}

	l, err := link.AttachTCX(link.TCXOptions{
		Interface: ifIndex,
		Program:   prog,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err != nil {
		prog.Close()
		return fmt.Errorf("attach tc drop program: %w", err)
	}

	m.programs[executionID] = &loadedProgram{prog: prog, link: l, spec: spec}
	m.logger.Info("eBPF tc drop loaded", "executionID", executionID, "ifIndex", ifIndex, "dropPercent", dropPercent)
	return nil
}

func (m *Manager) Unload(executionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	loaded, ok := m.programs[executionID]
	if !ok {
		return nil
	}

	if loaded.link != nil {
		loaded.link.Close()
	}
	if loaded.prog != nil {
		loaded.prog.Close()
	}
	delete(m.programs, executionID)
	m.logger.Info("eBPF program unloaded", "executionID", executionID)
	return nil
}

func (m *Manager) UnloadAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, loaded := range m.programs {
		if loaded.link != nil {
			loaded.link.Close()
		}
		if loaded.prog != nil {
			loaded.prog.Close()
		}
		delete(m.programs, id)
	}
	m.logger.Info("all eBPF programs unloaded")
}

func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.programs)
}
