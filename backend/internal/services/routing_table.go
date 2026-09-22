package services

import (
	"fmt"
	"sort"
	"strings"

	"sni-gateway/internal/models"
)

// RoutingTable resolves priority at compile time so nginx can use hostname
// hashes instead of scanning a regular expression for every suffix rule.
type RoutingTable struct {
	exact  map[string]*models.Rule
	suffix map[string]*models.Rule
	exits  map[uint]models.Exit
}

func preferredRule(a, b *models.Rule) *models.Rule {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if b.Priority > a.Priority || (b.Priority == a.Priority && b.ID < a.ID) {
		return b
	}
	return a
}

func NewRoutingTable(rules []models.Rule, exits []models.Exit) (*RoutingTable, error) {
	table := &RoutingTable{exact: make(map[string]*models.Rule), suffix: make(map[string]*models.Rule), exits: make(map[uint]models.Exit)}
	for _, exit := range exits {
		if exit.Enabled {
			table.exits[exit.ID] = exit
		}
	}
	for _, original := range rules {
		if !original.Enabled || original.ExitID == nil {
			continue
		}
		if _, ok := table.exits[*original.ExitID]; !ok {
			continue
		}
		r := original
		r.Pattern = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.Pattern), "."))
		if !ValidDomain(r.Pattern) {
			return nil, fmt.Errorf("rule %d has invalid domain %q", r.ID, r.Pattern)
		}
		switch r.Type {
		case "exact":
			table.exact[r.Pattern] = preferredRule(table.exact[r.Pattern], &r)
		case "suffix":
			table.suffix[r.Pattern] = preferredRule(table.suffix[r.Pattern], &r)
		default:
			return nil, fmt.Errorf("rule %d has invalid type %q", r.ID, r.Type)
		}
	}
	return table, nil
}

func (t *RoutingTable) matchSuffix(host string) *models.Rule {
	var best *models.Rule
	for host != "" {
		best = preferredRule(best, t.suffix[host])
		_, rest, found := strings.Cut(host, ".")
		if !found {
			break
		}
		host = rest
	}
	return best
}

// Match is bounded by the number of labels in the hostname, not the list size.
func (t *RoutingTable) Match(host string) *models.Rule {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return preferredRule(t.exact[host], t.matchSuffix(host))
}

func (t *RoutingTable) MapLines() []string {
	domains := make(map[string]struct{}, len(t.exact)+len(t.suffix))
	for host := range t.exact {
		domains[host] = struct{}{}
	}
	for host := range t.suffix {
		domains[host] = struct{}{}
	}
	keys := make([]string, 0, len(domains))
	for host := range domains {
		keys = append(keys, host)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)*2)
	for _, host := range keys {
		winner := t.Match(host)
		lines = append(lines, fmt.Sprintf("%s %s;", host, backendAddr(t.exits[*winner.ExitID])))
		// A shadowed suffix must not override its higher-priority ancestor in nginx.
		if own := t.suffix[host]; own != nil && t.matchSuffix(host) == own {
			lines = append(lines, fmt.Sprintf("*.%s %s;", host, backendAddr(t.exits[*own.ExitID])))
		}
	}
	return lines
}
