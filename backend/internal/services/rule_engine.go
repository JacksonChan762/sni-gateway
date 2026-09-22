package services

import "strings"

type ParsedRule struct {
	Pattern string
	Type    string // exact / suffix
}

// ParseListFile parses domain lists, preserving suffix semantics and first-seen order.
func ParseListFile(content string) []ParsedRule {
	var rules []ParsedRule
	seen := make(map[ParsedRule]struct{})
	// Cut processes lines without Scanner token limits or an allocation per line.
	content = strings.TrimPrefix(content, "\ufeff")
	for content != "" {
		var line string
		line, content, _ = strings.Cut(content, "\n")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind := "exact"
		if strings.Contains(line, ",") {
			fields := strings.SplitN(line, ",", 3)
			switch strings.TrimSpace(fields[0]) {
			case "DOMAIN":
			case "DOMAIN-SUFFIX":
				kind = "suffix"
			default:
				continue
			}
			line = strings.TrimSpace(fields[1])
		}
		if strings.HasPrefix(line, "+.") {
			kind = "suffix"
			line = strings.TrimPrefix(line, "+.")
		}
		line = strings.ToLower(strings.TrimSuffix(line, "."))
		if !ValidDomain(line) {
			continue
		}
		rule := ParsedRule{Pattern: line, Type: kind}
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		rules = append(rules, rule)
	}
	return rules
}

func ValidDomain(domain string) bool {
	if len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
