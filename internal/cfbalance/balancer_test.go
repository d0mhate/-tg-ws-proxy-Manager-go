package cfbalance

import (
	"reflect"
	"testing"
)

func TestDomainsForDCIsStickyPerDC(t *testing.T) {
	var balancer Balancer
	domains := []string{"d1.example.com", "d2.example.com", "d3.example.com"}

	got1 := balancer.DomainsForDC(2, domains, true)
	got2 := balancer.DomainsForDC(2, domains, true)
	got3 := balancer.DomainsForDC(2, domains, true)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !reflect.DeepEqual(got1, want) {
		t.Fatalf("unexpected first domain order: got %v want %v", got1, want)
	}
	if !reflect.DeepEqual(got2, got1) {
		t.Fatalf("expected sticky domain order for same dc, got %v then %v", got1, got2)
	}
	if !reflect.DeepEqual(got3, got1) {
		t.Fatalf("expected sticky domain order for same dc, got %v then %v", got1, got3)
	}
}

func TestDomainsForDCAssignsDifferentDCsIndependently(t *testing.T) {
	var balancer Balancer
	domains := []string{"d1.example.com", "d2.example.com", "d3.example.com"}

	gotDC2 := balancer.DomainsForDC(2, domains, true)
	gotDC4 := balancer.DomainsForDC(4, domains, true)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !reflect.DeepEqual(gotDC2, want) {
		t.Fatalf("unexpected dc2 domain order: got %v want %v", gotDC2, want)
	}
	if want := []string{"d2.example.com", "d1.example.com", "d3.example.com"}; !reflect.DeepEqual(gotDC4, want) {
		t.Fatalf("unexpected dc4 domain order: got %v want %v", gotDC4, want)
	}
}

func TestDomainsForDCUsesPerInstanceState(t *testing.T) {
	var first Balancer
	var second Balancer
	domains := []string{"d1.example.com", "d2.example.com", "d3.example.com"}

	_ = first.DomainsForDC(2, domains, true)
	got := second.DomainsForDC(2, domains, true)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected independent domain order: got %v want %v", got, want)
	}
}
