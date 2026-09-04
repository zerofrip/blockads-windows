package tunnel

import (
	"strings"
	"testing"
)

// Reference implementation mirroring FilterListRepository.kt checkDomainAndParents /
// hasCustomRule / isBlocked / getBlockReason (custom/whitelist sets only).
//
// Precedence for HasCustomRule:
//  1. custom allow → 0
//  2. whitelist → 0
//  3. custom block → 1
//  4. else → -1
//
// Parent walk for query D: D; then for each parent P of D: P, then "*."+P.
// Domains in sets are stored lowercased (Kotlin loadCustomRules/loadWhitelist).
// Engine lowercases and strips trailing dots BEFORE calling DomainChecker;
// the repository itself does not strip trailing dots.

func refCheckDomainAndParents(domain string, contains func(string) bool) bool {
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

type refRuleSets struct {
	allow     map[string]struct{}
	block     map[string]struct{}
	whitelist map[string]struct{}
}

func (r refRuleSets) has(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

func (r refRuleSets) HasCustomRule(domain string) int {
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.allow, s) }) {
		return 0
	}
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.whitelist, s) }) {
		return 0
	}
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.block, s) }) {
		return 1
	}
	return -1
}

func (r refRuleSets) IsBlocked(domain string) bool {
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.allow, s) }) {
		return false
	}
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.block, s) }) {
		return true
	}
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.whitelist, s) }) {
		return false
	}
	return false
}

func (r refRuleSets) GetBlockReason(domain string) string {
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.allow, s) }) {
		return ""
	}
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.block, s) }) {
		return "CUSTOM_RULE"
	}
	if refCheckDomainAndParents(domain, func(s string) bool { return r.has(r.whitelist, s) }) {
		return ""
	}
	return ""
}

func setOf(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, i := range items {
		m[strings.ToLower(i)] = struct{}{}
	}
	return m
}

func TestCustomRulePrecedenceCharacterization(t *testing.T) {
	ref := refRuleSets{
		allow:     setOf("allowed.com", "*.allow-wild.com"),
		block:     setOf("blocked.com", "*.block-wild.com"),
		whitelist: setOf("white.com"),
	}
	both := refRuleSets{
		allow: setOf("conflict.com"),
		block: setOf("conflict.com"),
	}

	type tc struct {
		rules    refRuleSets
		domain   string
		override int
		blocked  bool
		reason   string
	}
	tests := []tc{
		{ref, "allowed.com", 0, false, ""},
		{ref, "foo.allowed.com", 0, false, ""},
		{ref, "x.allow-wild.com", 0, false, ""},
		// Wildcard rule *.allow-wild.com is only probed as "*."+parent while walking;
		// apex allow-wild.com is not matched by that rule alone.
		{ref, "allow-wild.com", -1, false, ""},
		{ref, "blocked.com", 1, true, "CUSTOM_RULE"},
		{ref, "a.blocked.com", 1, true, "CUSTOM_RULE"},
		{ref, "z.block-wild.com", 1, true, "CUSTOM_RULE"},
		{ref, "white.com", 0, false, ""},
		{ref, "a.white.com", 0, false, ""},
		{ref, "example.org", -1, false, ""},
		// Engine lowercases before call; repository sets are lowercase.
		// If a caller passed uppercase without normalizing, Kotlin contains would miss —
		// we characterize the engine contract (lowercased input) here:
		{ref, "blocked.com", 1, true, "CUSTOM_RULE"},
		{both, "conflict.com", 0, false, ""}, // allow before block
		{ref, "", -1, false, ""},
		// Trailing dot: repository does not strip; engine does before DomainChecker.
		{ref, "blocked.com.", -1, false, ""},
	}

	for _, tt := range tests {
		if got := tt.rules.HasCustomRule(tt.domain); got != tt.override {
			t.Errorf("HasCustomRule(%q)=%d want %d", tt.domain, got, tt.override)
		}
		if got := tt.rules.IsBlocked(tt.domain); got != tt.blocked {
			t.Errorf("IsBlocked(%q)=%v want %v", tt.domain, got, tt.blocked)
		}
		if got := tt.rules.GetBlockReason(tt.domain); got != tt.reason {
			t.Errorf("GetBlockReason(%q)=%q want %q", tt.domain, got, tt.reason)
		}
	}
}

func TestCustomRuleParentWildcardWalkOrder(t *testing.T) {
	// Query sub.ads.example.com checks:
	//   sub.ads.example.com
	//   ads.example.com, *.ads.example.com
	//   example.com, *.example.com
	//   com, *.com
	ref := refRuleSets{block: setOf("*.example.com")}
	if ref.HasCustomRule("foo.example.com") != 1 {
		t.Fatal("expected wildcard parent block")
	}
	if ref.HasCustomRule("example.com") != -1 {
		t.Fatal("apex must not match *.example.com via parent walk alone")
	}
}
