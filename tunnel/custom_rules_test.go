package tunnel

import "testing"

func TestCustomRuleCheckerMatchesCharacterization(t *testing.T) {
	ref := refRuleSets{
		allow:     setOf("allowed.com", "*.allow-wild.com"),
		block:     setOf("blocked.com", "*.block-wild.com"),
		whitelist: setOf("white.com"),
	}
	got := NewCustomRuleChecker()
	got.SetRules(
		[]string{"allowed.com", "*.allow-wild.com"},
		[]string{"blocked.com", "*.block-wild.com"},
		[]string{"white.com"},
	)

	domains := []string{
		"allowed.com", "foo.allowed.com", "x.allow-wild.com", "allow-wild.com",
		"blocked.com", "a.blocked.com", "z.block-wild.com",
		"white.com", "a.white.com", "example.org", "", "blocked.com.",
	}
	for _, d := range domains {
		if g, w := got.HasCustomRule(d), ref.HasCustomRule(d); g != w {
			t.Errorf("HasCustomRule(%q): got %d want %d", d, g, w)
		}
		if g, w := got.IsBlocked(d), ref.IsBlocked(d); g != w {
			t.Errorf("IsBlocked(%q): got %v want %v", d, g, w)
		}
		if g, w := got.GetBlockReason(d), ref.GetBlockReason(d); g != w {
			t.Errorf("GetBlockReason(%q): got %q want %q", d, g, w)
		}
	}

	// allow beats block
	got.SetRules([]string{"conflict.com"}, []string{"conflict.com"}, nil)
	if got.HasCustomRule("conflict.com") != 0 {
		t.Fatal("allow must win over block")
	}
}
