package services

import (
	"fmt"
	"math/rand"
	"sni-gateway/internal/models"
	"strings"
	"testing"
)

func routingFixtures() ([]models.Rule, []models.Exit) {
	a, b := uint(1), uint(2)
	return []models.Rule{
		{ID: 1, Pattern: "example.com", Type: "suffix", Priority: 100, ExitID: &a, Enabled: true},
		{ID: 2, Pattern: "api.example.com", Type: "exact", Priority: 50, ExitID: &b, Enabled: true},
		{ID: 3, Pattern: "deep.example.com", Type: "suffix", Priority: 50, ExitID: &b, Enabled: true},
		{ID: 4, Pattern: "special.example.com", Type: "suffix", Priority: 200, ExitID: &b, Enabled: true},
		{ID: 5, Pattern: "only.example.net", Type: "exact", Priority: 100, ExitID: &b, Enabled: true},
		{ID: 6, Pattern: "example.com", Type: "suffix", Priority: 100, ExitID: &b, Enabled: true},
	}, []models.Exit{{ID: 1, Name: "a", Enabled: true}, {ID: 2, Name: "b", Enabled: true}}
}

func TestRoutingPriorityAndBoundaries(t *testing.T) {
	rules, exits := routingFixtures()
	table, err := NewRoutingTable(rules, exits)
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]uint{"example.com": 1, "API.Example.COM.": 1, "x.deep.example.com": 1, "special.example.com": 4, "x.special.example.com": 4, "only.example.net": 5, "x.only.example.net": 0, "notexample.com": 0} {
		got := table.Match(host)
		if want == 0 {
			if got != nil {
				t.Fatalf("%s unexpectedly matched", host)
			}
			continue
		}
		if got == nil || got.ID != want {
			t.Fatalf("%s: got %#v, want rule %d", host, got, want)
		}
	}
	lines := strings.Join(table.MapLines(), "\n")
	if strings.Contains(lines, "~") || strings.Contains(lines, "*.deep.example.com") {
		t.Fatalf("unexpected slow or shadowed match: %s", lines)
	}
	if strings.Count(lines, "\nexample.com ") > 1 {
		t.Fatal("duplicate key")
	}
}

func TestRoutingMapAgreesWithReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	exits := []models.Exit{{ID: 1, Name: "a", Enabled: true}, {ID: 2, Name: "b", Enabled: true}}
	var rules []models.Rule
	for i := 0; i < 1500; i++ {
		kind := "exact"
		if rng.Intn(2) == 0 {
			kind = "suffix"
		}
		pattern := fmt.Sprintf("d%d.example.com", rng.Intn(50))
		if rng.Intn(2) == 0 {
			pattern = "sub." + pattern
		}
		exitID := uint(1 + i%2)
		rules = append(rules, models.Rule{ID: uint(i + 1), Pattern: pattern, Type: kind, Priority: rng.Intn(20), ExitID: &exitID, Enabled: true})
	}
	table, err := NewRoutingTable(rules, exits)
	if err != nil {
		t.Fatal(err)
	}
	lines := table.MapLines()
	for i := 0; i < 1000; i++ {
		host := fmt.Sprintf("d%d.example.com", rng.Intn(60))
		if rng.Intn(2) == 0 {
			host = "sub." + host
		}
		if rng.Intn(2) == 0 {
			host = "x." + host
		}
		var want *models.Rule
		for j := range rules {
			r := &rules[j]
			if host == r.Pattern || r.Type == "suffix" && strings.HasSuffix(host, "."+r.Pattern) {
				if want == nil || r.Priority > want.Priority || r.Priority == want.Priority && r.ID < want.ID {
					want = r
				}
			}
		}
		got := table.Match(host)
		if (got == nil) != (want == nil) || got != nil && got.ID != want.ID {
			t.Fatalf("reference mismatch for %s", host)
		}
		expected := "127.0.0.1:10099"
		if want != nil {
			expected = backendAddr(exits[int(*want.ExitID)-1])
		}
		if compiled := evaluateHostnameMap(lines, host); compiled != expected {
			t.Fatalf("compiled map for %s: got %s want %s", host, compiled, expected)
		}

	}
}

func TestRoutingRejectsInvalidAndSkipsInactive(t *testing.T) {
	rules, exits := routingFixtures()
	rules[0].Pattern = "bad.example;"
	if _, err := NewRoutingTable(rules, exits); err == nil {
		t.Fatal("invalid domain accepted")
	}
	rules[0].Enabled = false
	if _, err := NewRoutingTable(rules, exits); err != nil {
		t.Fatal(err)
	}
	exits[0].Enabled = false
	table, err := NewRoutingTable(rules, exits)
	if err != nil {
		t.Fatal(err)
	}
	if got := table.Match("api.example.com"); got == nil || *got.ExitID != 2 {
		t.Fatal("inactive exit was selected")
	}
}

func BenchmarkRoutingMatch(b *testing.B) {
	for _, size := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			id := uint(1)
			rules := make([]models.Rule, size)
			for i := range rules {
				rules[i] = models.Rule{ID: uint(i + 1), Pattern: fmt.Sprintf("d%d.example.com", i), Type: "suffix", ExitID: &id, Enabled: true}
			}
			table, err := NewRoutingTable(rules, []models.Exit{{ID: 1, Name: "a", Enabled: true}})
			if err != nil {
				b.Fatal(err)
			}
			host := fmt.Sprintf("sub.d%d.example.com", size-1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if table.Match(host) == nil {
					b.Fatal("missing match")
				}
			}
		})
	}
}

// Independent evaluator for nginx's exact-then-longest-wildcard semantics.
func evaluateHostnameMap(lines []string, host string) string {
	best, length := "127.0.0.1:10099", 0
	for _, line := range lines {
		fields := strings.Fields(line)
		key, value := fields[0], strings.TrimSuffix(fields[1], ";")
		if key == host {
			return value
		}
		if strings.HasPrefix(key, "*.") && strings.HasSuffix(host, key[1:]) && len(key) > length {
			best, length = value, len(key)
		}
	}
	return best
}
