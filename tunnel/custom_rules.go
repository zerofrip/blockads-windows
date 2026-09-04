package tunnel

import (
	"strings"
	"sync"
)

const customBlockReason = "CUSTOM_RULE"

// CustomRuleChecker is a portable DomainChecker for custom allow/block rules
// and whitelist domains. Precedence matches FilterListRepository.kt:
//
//	allow → whitelist → block → none
//
// Android may continue using the Kotlin DomainChecker; Windows uses this type.
type CustomRuleChecker struct {
	mu        sync.RWMutex
	allow     map[string]struct{}
	block     map[string]struct{}
	whitelist map[string]struct{}
}

// NewCustomRuleChecker returns an empty checker.
func NewCustomRuleChecker() *CustomRuleChecker {
	return &CustomRuleChecker{
		allow:     make(map[string]struct{}),
		block:     make(map[string]struct{}),
		whitelist: make(map[string]struct{}),
	}
}

// SetRules replaces all rule sets. Domains are normalized to lowercase.
// Trailing dots are not stripped here (engine strips before DomainChecker calls).
func (c *CustomRuleChecker) SetRules(allow, block, whitelist []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allow = toDomainSet(allow)
	c.block = toDomainSet(block)
	c.whitelist = toDomainSet(whitelist)
}

func toDomainSet(domains []string) map[string]struct{} {
	m := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		m[d] = struct{}{}
	}
	return m
}

func checkDomainAndParents(domain string, contains func(string) bool) bool {
	if contains(domain) {
		return true
	}
	d := domain
	for strings.Contains(d, ".") {
		d = d[strings.IndexByte(d, '.')+1:]
		if contains(d) {
			return true
		}
		if contains("*." + d) {
			return true
		}
	}
	return false
}

func (c *CustomRuleChecker) hasIn(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// HasCustomRule implements DomainChecker. Returns 0 allow, 1 block, -1 none.
func (c *CustomRuleChecker) HasCustomRule(domain string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.allow, s) }) {
		return 0
	}
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.whitelist, s) }) {
		return 0
	}
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.block, s) }) {
		return 1
	}
	return -1
}

// IsBlocked implements DomainChecker (custom/whitelist only; tries are separate).
func (c *CustomRuleChecker) IsBlocked(domain string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.allow, s) }) {
		return false
	}
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.block, s) }) {
		return true
	}
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.whitelist, s) }) {
		return false
	}
	return false
}

// GetBlockReason implements DomainChecker.
func (c *CustomRuleChecker) GetBlockReason(domain string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.allow, s) }) {
		return ""
	}
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.block, s) }) {
		return customBlockReason
	}
	if checkDomainAndParents(domain, func(s string) bool { return c.hasIn(c.whitelist, s) }) {
		return ""
	}
	return ""
}

// Ensure CustomRuleChecker implements DomainChecker at compile time.
var _ DomainChecker = (*CustomRuleChecker)(nil)
