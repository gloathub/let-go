package vm

import "testing"

func TestProtocolIsMetadataAware(t *testing.T) {
	p := NewProtocol("P", []Symbol{Symbol("foo")})
	var _ IMeta = p // Protocol must implement IMeta
	if p.Meta() != NIL {
		t.Fatalf("fresh protocol Meta() = %v, want NIL", p.Meta())
	}
	m := String("the-meta")
	p2 := p.WithMeta(m).(*Protocol)
	if p2.Meta() != Value(m) {
		t.Fatalf("WithMeta did not attach meta: %v", p2.Meta())
	}
	if p.Meta() != NIL {
		t.Fatalf("WithMeta mutated the original")
	}
	if p2.Name() != "P" || len(p2.Methods()) != 1 {
		t.Fatalf("WithMeta lost protocol identity/dispatch data")
	}
}
