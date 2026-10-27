package sparql

import (
	"context"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
)

func TestComplexQuestionsHaveVerifiedEvidence(t *testing.T) {
	g := domain.ChallengeGraph()
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 99 || len(g.Edges) != 295 || len(g.Subgraphs) != 6 {
		t.Fatal("unexpected challenge fixture size")
	}
	counts := []int{4, 8, 1, 2, 1, 8}
	for i, example := range g.Examples {
		result, err := Evaluate(context.Background(), g, example.Query)
		if err != nil {
			t.Fatalf("question %d: %v", i+1, err)
		}
		if len(result.Rows) != counts[i] {
			t.Fatalf("question %d: got %d rows, want %d: %+v", i+1, len(result.Rows), counts[i], result.Rows)
		}
		switch i {
		case 0:
			for j, id := range []string{"r01", "r09", "r13", "r21"} {
				if result.Rows[j]["researcher"].Value != domain.Namespace+id {
					t.Fatal("wrong climate-project researcher", result.Rows)
				}
			}
		case 1:
			for j, count := range []string{"2", "2", "3", "4", "2", "3", "2", "2"} {
				if result.Rows[j]["countries"].Value != count {
					t.Fatal("wrong distinct-country count", result.Rows)
				}
			}
		case 2:
			if result.Rows[0]["researcher"].Value != domain.Namespace+"r08" {
				t.Fatal("wrong researcher for negative condition", result.Rows)
			}
		case 3:
			if result.Rows[0]["lab"].Value != domain.Namespace+"lab0" || result.Rows[1]["lab"].Value != domain.Namespace+"lab4" {
				t.Fatal("wrong transitive partnership results", result.Rows)
			}
		case 4:
			if result.Rows[0]["project"].Value != domain.Namespace+"project05" || result.Rows[0]["totalCitations"].Value != "100" {
				t.Fatal("citations were not deduplicated", result.Rows)
			}
		case 5:
			for j, id := range []string{"r01", "r04", "r07", "r10", "r13", "r16", "r19", "r22"} {
				if result.Rows[j]["researcher"].Value != domain.Namespace+id {
					t.Fatal("wrong cross-country coauthor", result.Rows)
				}
			}
		}
	}
}

// A wrong author direction must not pass just because the base fixture has one
// matching leader. Add a retracted author who otherwise meets every condition.
func TestNegativeChallengeExcludesRetractedLeader(t *testing.T) {
	g := domain.ChallengeGraph()
	g.Edges = append(g.Edges, domain.Edge{ID: "adversarial-leader", Source: domain.Namespace + "r24", Predicate: domain.Namespace + "leads", Label: "leads", Target: domain.Namespace + "project08"})
	result, err := Evaluate(context.Background(), g, g.Examples[2].Query)
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["researcher"].Value != domain.Namespace+"r08" {
		t.Fatal("retracted leader was not excluded", result.Rows, err)
	}
}
