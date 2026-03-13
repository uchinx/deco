package example

import "fmt"

// MyStruct has both used and unused methods.
type MyStruct struct {
	Name string
}

// UsedMethod is called in main, should NOT be reported.
func (m *MyStruct) UsedMethod() string {
	return m.Name
}

// UnusedMethod is never called, should be reported as dead code.
func (m *MyStruct) UnusedMethod() int {
	return 42
}

// String satisfies fmt.Stringer, should NOT be reported.
func (m *MyStruct) String() string {
	return fmt.Sprintf("MyStruct{%s}", m.Name)
}

// Error satisfies the error interface, should NOT be reported.
func (m *MyStruct) Error() string {
	return "error: " + m.Name
}

// MyInterface has both used and unused methods.
type MyInterface interface {
	DoSomething() error
	NeverCalled() string
}

// implStruct implements MyInterface.
type implStruct struct{}

func (i *implStruct) DoSomething() error {
	return nil
}

func (i *implStruct) NeverCalled() string {
	return "never"
}

// unexportedMethod should NOT be reported (not exported).
func (m *MyStruct) unexportedMethod() {}

// AnotherUnused is on a value receiver, never called.
func (m MyStruct) AnotherUnused() bool {
	return true
}

// Worker interface — Run is called on the concrete type, NOT through the interface.
// Should NOT be reported as dead code.
type Worker interface {
	Run() error
}

// ConcreteWorker implements Worker.
type ConcreteWorker struct{}

func (c *ConcreteWorker) Run() error {
	return nil
}

// Signer interface — Sign is called through the interface only.
// The concrete struct method should NOT be reported as dead code.
type Signer interface {
	Sign(data string) string
}

// ConcreteSigner implements Signer.
type ConcreteSigner struct{}

func (c *ConcreteSigner) Sign(data string) string {
	return "signed:" + data
}

// OrderProcessor has a method only "used" by a mock in a test file.
// Should be reported as dead code.
type OrderProcessor interface {
	ProcessBulk(count int) error
}

// RealProcessor implements OrderProcessor.
type RealProcessor struct{}

func (r *RealProcessor) ProcessBulk(count int) error {
	return nil
}

func UseThings() {
	s := &MyStruct{Name: "test"}
	_ = s.UsedMethod()

	var iface MyInterface = &implStruct{}
	_ = iface.DoSomething()

	// Call Run on concrete type, not through Worker interface.
	w := &ConcreteWorker{}
	_ = w.Run()

	// Call Sign through interface, not on concrete type.
	var signer Signer = &ConcreteSigner{}
	_ = signer.Sign("hello")
}
