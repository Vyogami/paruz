package backend

import "testing"

func TestRankMatches(t *testing.T) {
	cands := []string{
		"neon", "neoss", "nexus-oss", "newsboat-og", "neoleo",
		"neodlp", "neofetch", "neo", "go-neon", "telegram-neo",
	}

	t.Run("prefix matches rank above scattered matches", func(t *testing.T) {
		got := rankMatches([]string{"neo"}, cands)
		if len(got) == 0 {
			t.Fatal("expected matches, got none")
		}
		// Exact match wins.
		if got[0].Name != "neo" {
			t.Errorf("expected first result 'neo', got %q", got[0].Name)
		}

		pos := map[string]int{}
		for i, p := range got {
			pos[p.Name] = i
		}
		// Prefix matches must rank above the scattered 'nexus-oss' match.
		for _, prefix := range []string{"neon", "neoss", "neofetch"} {
			if _, ok := pos[prefix]; !ok {
				t.Fatalf("expected %q in results", prefix)
			}
			if nx, ok := pos["nexus-oss"]; ok && pos[prefix] > nx {
				t.Errorf("expected prefix match %q (idx %d) to rank above nexus-oss (idx %d)", prefix, pos[prefix], nx)
			}
		}
		// Closer prefix match ranks above a longer one.
		if pos["neon"] > pos["neofetch"] {
			t.Errorf("expected 'neon' (idx %d) above 'neofetch' (idx %d)", pos["neon"], pos["neofetch"])
		}
	})

	t.Run("empty terms returns nil", func(t *testing.T) {
		if got := rankMatches(nil, cands); got != nil {
			t.Errorf("expected nil for empty terms, got %v", got)
		}
	})

	t.Run("multi-term requires all terms", func(t *testing.T) {
		multi := []string{"python-requests", "python-flask", "ruby-rails"}
		got := rankMatches([]string{"python", "req"}, multi)
		if len(got) != 1 || got[0].Name != "python-requests" {
			t.Errorf("expected only 'python-requests', got %v", got)
		}
	})

	t.Run("non-matching query returns nothing", func(t *testing.T) {
		if got := rankMatches([]string{"zzzqqq"}, cands); len(got) != 0 {
			t.Errorf("expected no matches, got %v", got)
		}
	})
}
