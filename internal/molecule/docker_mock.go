package molecule

import (
	"context"
	"fmt"
)

// DockerInterface defines the interface for Docker operations
type DockerInterface interface {
	Run(ctx context.Context, args ...string) error
	Exec(ctx context.Context, container string, args ...string) error
	Pull(ctx context.Context, image string) error
	Stop(ctx context.Context, container string) error
	Remove(ctx context.Context, container string) error
	ImageExists(ctx context.Context, image string) bool
	ContainerExists(ctx context.Context, container string) bool
}

// RealDocker implements DockerInterface using actual docker commands
type RealDocker struct{}

func (d *RealDocker) Run(ctx context.Context, args ...string) error {
	// Implementation would call actual docker run
	return fmt.Errorf("docker not available in test environment")
}

func (d *RealDocker) Exec(ctx context.Context, container string, args ...string) error {
	// Implementation would call actual docker exec
	return fmt.Errorf("docker not available in test environment")
}

func (d *RealDocker) Pull(ctx context.Context, image string) error {
	// Implementation would call actual docker pull
	return fmt.Errorf("docker not available in test environment")
}

func (d *RealDocker) Stop(ctx context.Context, container string) error {
	// Implementation would call actual docker stop
	return fmt.Errorf("docker not available in test environment")
}

func (d *RealDocker) Remove(ctx context.Context, container string) error {
	// Implementation would call actual docker rm
	return fmt.Errorf("docker not available in test environment")
}

func (d *RealDocker) ImageExists(ctx context.Context, image string) bool {
	// Implementation would check if image exists
	return false
}

func (d *RealDocker) ContainerExists(ctx context.Context, container string) bool {
	// Implementation would check if container exists
	return false
}

// MockDocker implements DockerInterface for testing
type MockDocker struct {
	RunCalls       [][]string
	ExecCalls      [][]string
	PullCalls      []string
	StopCalls      []string
	RemoveCalls    []string
	ShouldFail     bool
	ImageExistsMap map[string]bool
	ContainerMap   map[string]bool
}

func NewMockDocker() *MockDocker {
	return &MockDocker{
		ImageExistsMap: make(map[string]bool),
		ContainerMap:   make(map[string]bool),
	}
}

func (m *MockDocker) Run(ctx context.Context, args ...string) error {
	m.RunCalls = append(m.RunCalls, args)
	if m.ShouldFail {
		return fmt.Errorf("mock docker run failed")
	}
	return nil
}

func (m *MockDocker) Exec(ctx context.Context, container string, args ...string) error {
	m.ExecCalls = append(m.ExecCalls, append([]string{container}, args...))
	if m.ShouldFail {
		return fmt.Errorf("mock docker exec failed")
	}
	return nil
}

func (m *MockDocker) Pull(ctx context.Context, image string) error {
	m.PullCalls = append(m.PullCalls, image)
	if m.ShouldFail {
		return fmt.Errorf("mock docker pull failed")
	}
	return nil
}

func (m *MockDocker) Stop(ctx context.Context, container string) error {
	m.StopCalls = append(m.StopCalls, container)
	if m.ShouldFail {
		return fmt.Errorf("mock docker stop failed")
	}
	return nil
}

func (m *MockDocker) Remove(ctx context.Context, container string) error {
	m.RemoveCalls = append(m.RemoveCalls, container)
	if m.ShouldFail {
		return fmt.Errorf("mock docker remove failed")
	}
	return nil
}

func (m *MockDocker) ImageExists(ctx context.Context, image string) bool {
	exists, ok := m.ImageExistsMap[image]
	return ok && exists
}

func (m *MockDocker) ContainerExists(ctx context.Context, container string) bool {
	exists, ok := m.ContainerMap[container]
	return ok && exists
}

// Global docker instance - can be replaced for testing
var dockerClient DockerInterface = &RealDocker{}