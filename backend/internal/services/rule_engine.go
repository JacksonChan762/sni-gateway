package services

import (
	"bufio"
	"strings"
)

type ParsedRule struct {
	Pattern string
	Type    string // exact / suffix
}

// ParseListFile parses a .list file and returns rules
func ParseListFile(content string) []ParsedRule {
	var rules []ParsedRule
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Handle DOMAIN, DOMAIN-SUFFIX prefixes from clash/meta format
		line = strings.TrimPrefix(line, "DOMAIN,")
		line = strings.TrimPrefix(line, "DOMAIN-SUFFIX,")
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "+.") {
			rules = append(rules, ParsedRule{
				Pattern: line[2:], // strip "+."
				Type:    "suffix",
			})
		} else if strings.Contains(line, ".") {
			rules = append(rules, ParsedRule{
				Pattern: line,
				Type:    "exact",
			})
		}
	}
	return rules
}
