// job-search-loop demonstrates LOOP ENGINEERING on top of the basic
// examples/job-search pipeline: instead of a straight-line pass, the graph
// routes back on itself until a measurable quality bar is met.
//
// Two engineered loops:
//
//	          ┌──────────────┐
//	          ▼              │ too few results
//	search ──gate──► refine ─┘   (loop 1: query-refinement)
//	          │ enough results
//	          ▼
//	       analyze ──► generate
//	                      │
//	          ┌───────────▼───────────┐
//	          │        critique       │   (loop 2: evaluator-optimizer)
//	          │           │           │
//	          │     below threshold   │
//	          ▼           │           │
//	        revise ───────┘           │
//	                      │ all docs ≥ threshold or max revisions
//	                      ▼
//	                    submit
//
// Loop 1 fixes a brittle input problem: a narrow query returns too few jobs,
// so a refine node broadens it and searches again — bounded by maxSearchAttempts.
//
// Loop 2 is the evaluator-optimizer pattern: a critic node scores every
// generated document; documents below the quality threshold are revised and
// re-scored — bounded by maxRevisions. The final output is measurably better
// than the first draft, and the bound guarantees termination.
//
// Both loops use plain graph.Router functions reading loop counters kept in
// the typed state — no framework magic, just cyclic routing.
//
// Replace the stubs with LLM calls for a production workflow:
//
//	provider, _ := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o"})
//
// Usage:
//
//	go run ./examples/job-search-loop
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

const (
	minResults        = 3  // loop 1: search until at least this many listings
	maxSearchAttempts = 3  // loop 1: bound
	qualityThreshold  = 85 // loop 2: every doc must score at least this
	maxRevisions      = 3  // loop 2: bound
)

type jobListing struct {
	ID      string
	Title   string
	Company string
	Tags    []string
}

type appDoc struct {
	JobID    string
	Resume   string
	Score    int // critic score 0–100
	Revision int // how many times this doc was revised
}

type state struct {
	Query          string
	SearchAttempts int
	Jobs           []jobListing
	Docs           []appDoc
	Revisions      int
	Submitted      []string
	Log            []string // human-readable trace of loop decisions
}

type S = core.State[state]

func main() {
	g := graph.New[state]("job-search-loop").
		AddNode(graph.Node[state]{
			ID:       "search",
			Runnable: searchNode(),
			// Loop 1 gate: enough results → analyze; too few → refine (bounded).
			Router: func(_ context.Context, s S) (string, error) {
				if len(s.Data.Jobs) >= minResults || s.Data.SearchAttempts >= maxSearchAttempts {
					return "analyze", nil
				}
				return "refine", nil
			},
		}).
		AddNode(graph.Node[state]{
			ID:       "refine",
			Runnable: refineNode(),
			Router:   graph.StaticRouter[state]("search"), // back edge: loop 1
		}).
		AddNode(graph.Node[state]{
			ID:       "analyze",
			Runnable: analyzeNode(),
			Router:   graph.StaticRouter[state]("generate"),
		}).
		AddNode(graph.Node[state]{
			ID:       "generate",
			Runnable: generateNode(),
			Router:   graph.StaticRouter[state]("critique"),
		}).
		AddNode(graph.Node[state]{
			ID:       "critique",
			Runnable: critiqueNode(),
			// Loop 2 gate: any doc below threshold → revise (bounded); else submit.
			Router: func(_ context.Context, s S) (string, error) {
				if worstScore(s.Data.Docs) < qualityThreshold && s.Data.Revisions < maxRevisions {
					return "revise", nil
				}
				return "submit", nil
			},
		}).
		AddNode(graph.Node[state]{
			ID:       "revise",
			Runnable: reviseNode(),
			Router:   graph.StaticRouter[state]("critique"), // back edge: loop 2
		}).
		AddNode(graph.Node[state]{
			ID:       "submit",
			Runnable: submitNode(),
		}).
		SetEntry("search")

	out, err := g.Invoke(context.Background(), core.NewState(state{
		Query: "principal Go engineer, quantum blockchain, Antarctica", // deliberately too narrow
	}))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("Loop trace:")
	for _, line := range out.Data.Log {
		fmt.Printf("  %s\n", line)
	}
	fmt.Printf("\nFinal query:   %s\n", out.Data.Query)
	fmt.Printf("Jobs found:    %d (after %d search attempt(s))\n", len(out.Data.Jobs), out.Data.SearchAttempts)
	fmt.Printf("Revisions:     %d\n", out.Data.Revisions)
	fmt.Printf("Doc scores:    %s\n", scoreList(out.Data.Docs))
	fmt.Printf("Submitted:     %d application(s)\n", len(out.Data.Submitted))
}

// searchNode queries job boards. The stub returns more results the broader the
// query is (fewer comma-separated constraints), simulating real search recall.
func searchNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		s.Data.SearchAttempts++
		// ponytail: stub recall model — constraint count drives result count
		constraints := len(strings.Split(s.Data.Query, ","))
		all := []jobListing{
			{ID: "j1", Title: "Senior Go Engineer", Company: "Acme AI", Tags: []string{"go", "llm"}},
			{ID: "j2", Title: "Platform Engineer", Company: "Globex", Tags: []string{"go", "grpc"}},
			{ID: "j3", Title: "AI Backend Lead", Company: "Initech", Tags: []string{"go", "ai"}},
			{ID: "j4", Title: "Staff Engineer", Company: "Umbrella", Tags: []string{"go", "k8s"}},
		}
		n := len(all) - constraints // broader query → more matches
		if n < 0 {
			n = 0
		}
		s.Data.Jobs = all[:n]
		s.Data.Log = append(s.Data.Log,
			fmt.Sprintf("search #%d: query=%q → %d result(s)", s.Data.SearchAttempts, s.Data.Query, n))
		return s, nil
	})
}

// refineNode broadens the query by dropping its most restrictive constraint.
// Replace with an LLM call: "This query returned too few results. Broaden it."
func refineNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		parts := strings.Split(s.Data.Query, ",")
		if len(parts) > 1 {
			s.Data.Query = strings.TrimSpace(strings.Join(parts[:len(parts)-1], ","))
		}
		s.Data.Log = append(s.Data.Log, fmt.Sprintf("refine: broadened query to %q", s.Data.Query))
		return s, nil
	})
}

// analyzeNode filters listings to those worth applying to.
// Replace with advisor agents (see examples/job-search for the multi-advisor version).
func analyzeNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		kept := s.Data.Jobs[:0]
		for _, j := range s.Data.Jobs {
			for _, t := range j.Tags {
				if t == "go" {
					kept = append(kept, j)
					break
				}
			}
		}
		s.Data.Jobs = kept
		s.Data.Log = append(s.Data.Log, fmt.Sprintf("analyze: %d listing(s) pass fit filter", len(kept)))
		return s, nil
	})
}

// generateNode writes the first draft of each application document.
// Replace with an LLM call that tailors a resume per job description.
func generateNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		for _, j := range s.Data.Jobs {
			s.Data.Docs = append(s.Data.Docs, appDoc{
				JobID:  j.ID,
				Resume: fmt.Sprintf("[draft resume for %s @ %s]", j.Title, j.Company),
			})
		}
		s.Data.Log = append(s.Data.Log, fmt.Sprintf("generate: %d first draft(s)", len(s.Data.Docs)))
		return s, nil
	})
}

// critiqueNode scores every document. First drafts score low; each revision
// raises the score, modelling an LLM critic that finds fewer issues each pass.
// Replace with an LLM call: "Score this resume against the job description,
// return score + concrete issues."
func critiqueNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		for i := range s.Data.Docs {
			// ponytail: stub critic — 70 base + 8 per revision, capped 100
			score := 70 + s.Data.Docs[i].Revision*8
			if score > 100 {
				score = 100
			}
			s.Data.Docs[i].Score = score
		}
		s.Data.Log = append(s.Data.Log,
			fmt.Sprintf("critique: scores=%s (threshold %d)", scoreList(s.Data.Docs), qualityThreshold))
		return s, nil
	})
}

// reviseNode rewrites only the documents that failed the quality bar.
// Replace with an LLM call that applies the critic's concrete issues.
func reviseNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		s.Data.Revisions++
		n := 0
		for i := range s.Data.Docs {
			if s.Data.Docs[i].Score < qualityThreshold {
				s.Data.Docs[i].Revision++
				s.Data.Docs[i].Resume = fmt.Sprintf("[rev %d] %s", s.Data.Docs[i].Revision, s.Data.Docs[i].Resume)
				n++
			}
		}
		s.Data.Log = append(s.Data.Log, fmt.Sprintf("revise #%d: rewrote %d doc(s) below threshold", s.Data.Revisions, n))
		return s, nil
	})
}

// submitNode posts every application that met the quality bar.
func submitNode() core.TypedRunnable[S, S] {
	return core.TypedFunc(func(_ context.Context, s S) (S, error) {
		for _, d := range s.Data.Docs {
			if d.Score >= qualityThreshold {
				s.Data.Submitted = append(s.Data.Submitted, d.JobID)
			}
		}
		s.Data.Log = append(s.Data.Log, fmt.Sprintf("submit: %d application(s) sent", len(s.Data.Submitted)))
		return s, nil
	})
}

func worstScore(docs []appDoc) int {
	worst := 100
	for _, d := range docs {
		if d.Score < worst {
			worst = d.Score
		}
	}
	return worst
}

func scoreList(docs []appDoc) string {
	parts := make([]string, len(docs))
	for i, d := range docs {
		parts[i] = fmt.Sprintf("%s=%d", d.JobID, d.Score)
	}
	return strings.Join(parts, " ")
}
