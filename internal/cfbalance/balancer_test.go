package cfbalance

import (
	"reflect"
	"testing"
)

func TestDomainsBalancesRoundRobin(t *testing.T) {
	var balancer Balancer
	domains := []string{"d1.example.com", "d2.example.com", "d3.example.com"}

	got1 := balancer.Domains(domains, true)
	got2 := balancer.Domains(domains, true)
	got3 := balancer.Domains(domains, true)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !reflect.DeepEqual(got1, want) {
		t.Fatalf("unexpected first domain order: got %v want %v", got1, want)
	}
	if want := []string{"d2.example.com", "d3.example.com", "d1.example.com"}; !reflect.DeepEqual(got2, want) {
		t.Fatalf("unexpected second domain order: got %v want %v", got2, want)
	}
	if want := []string{"d3.example.com", "d1.example.com", "d2.example.com"}; !reflect.DeepEqual(got3, want) {
		t.Fatalf("unexpected third domain order: got %v want %v", got3, want)
	}
}

func TestDomainsUsesPerInstanceState(t *testing.T) {
	var first Balancer
	var second Balancer
	domains := []string{"d1.example.com", "d2.example.com", "d3.example.com"}

	_ = first.Domains(domains, true)
	got := second.Domains(domains, true)

	if want := []string{"d1.example.com", "d2.example.com", "d3.example.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected independent domain order: got %v want %v", got, want)
	}
}
