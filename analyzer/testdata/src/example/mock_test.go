package example

import "testing"

// MockProcessor is a mock implementation of OrderProcessor.
type MockProcessor struct{}

func (m *MockProcessor) ProcessBulk(count int) error {
	return nil
}

func TestUseMock(t *testing.T) {
	var p OrderProcessor = &MockProcessor{}
	_ = p.ProcessBulk(5)
}

