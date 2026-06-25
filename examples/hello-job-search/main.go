// hello-job-search demonstrates a multi-agent job search workflow using pkg/graph.
//
// Pipeline:
//
//	search → analyze → summarize → generate → submit
//
// The analyze node runs three advisor agents (tech fit, culture fit, compensation)
// and produces a scored analysis for each listing. The generate node writes a
// tailored resume and cover letter per job. The submit node posts each application.
//
// Replace the stub implementations with real LLM providers and job-board API clients
// for a production workflow:
//
//	provider, _ := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o"})
//
// Usage:
//
//	go run ./examples/hello-job-search
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

type jobListing struct {
	ID       string
	Title    string
	Company  string
	Location string
	Tags     []string
}

type advisorScore struct {
	Advisor string
	Score   int // 0–100
	Reason  string
}

type jobAnalysis struct {
	JobID    string
	Scores   []advisorScore
	Fit      int // average of advisor scores
}

type applicationDocs struct {
	JobID       string
	Resume      string
	CoverLetter string
}

type jobSearchState struct {
	Query     string
	Jobs      []jobListing
	Analyses  []jobAnalysis
	Summary   string
	Docs      []applicationDocs
	Submitted []string
}

func main() {
	g := graph.New[jobSearchState]("job-search").
		AddNode(graph.Node[jobSearchState]{
			ID:       "search",
			Runnable: searchNode(),
			Router:   graph.StaticRouter[jobSearchState]("analyze"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "analyze",
			Runnable: analyzeNode(),
			Router:   graph.StaticRouter[jobSearchState]("summarize"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "summarize",
			Runnable: summarizeNode(),
			Router:   graph.StaticRouter[jobSearchState]("generate"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "generate",
			Runnable: generateNode(),
			Router:   graph.StaticRouter[jobSearchState]("submit"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "submit",
			Runnable: submitNode(),
		}).
		SetEntry("search")

	init := core.NewState(jobSearchState{Query: "senior Go engineer, remote, AI platform"})

	out, err := g.Invoke(context.Background(), init)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("Query:     %s\n", out.Data.Query)
	fmt.Printf("Jobs found: %d\n", len(out.Data.Jobs))
	fmt.Printf("Summary:\n%s\n", out.Data.Summary)
	fmt.Printf("Applications submitted: %d\n", len(out.Data.Submitted))
	for _, id := range out.Data.Submitted {
		fmt.Printf("  - %s\n", id)
	}
}

// searchNode queries job boards and returns matching listings.
// Replace the stub with real API clients (LinkedIn, Seek, Greenhouse, etc.).
func searchNode() core.TypedRunnable[core.State[jobSearchState], core.State[jobSearchState]] {
	return core.TypedFunc(func(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
		// ponytail: stub listings — replace with job-board API calls
		s.Data.Jobs = []jobListing{
			{ID: "j1", Title: "Senior Go Engineer", Company: "Acme AI", Location: "Remote", Tags: []string{"go", "kubernetes", "llm"}},
			{ID: "j2", Title: "Platform Engineer", Company: "Globex Cloud", Location: "Remote, SG", Tags: []string{"go", "grpc", "devops"}},
			{ID: "j3", Title: "AI Backend Lead", Company: "Initech Labs", Location: "Remote", Tags: []string{"go", "python", "ai"}},
		}
		return s, nil
	})
}

// analyzeNode runs three advisor agents (tech, culture, compensation) for each listing.
// Replace the scoring stubs with LLM calls that evaluate each job against a resume.
func analyzeNode() core.TypedRunnable[core.State[jobSearchState], core.State[jobSearchState]] {
	return core.TypedFunc(func(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
		scorers := []func(jobListing) advisorScore{techAdvisor, cultureAdvisor, compAdvisor}

		for _, job := range s.Data.Jobs {
			var scores []advisorScore
			total := 0
			for _, scorer := range scorers {
				sc := scorer(job)
				scores = append(scores, sc)
				total += sc.Score
			}
			s.Data.Analyses = append(s.Data.Analyses, jobAnalysis{
				JobID:  job.ID,
				Scores: scores,
				Fit:    total / len(scorers),
			})
		}
		return s, nil
	})
}

// summarizeNode synthesizes advisor analyses into a ranked human-readable summary.
// Replace the stub with an LLM call: "Given these analyses, produce a ranked summary."
func summarizeNode() core.TypedRunnable[core.State[jobSearchState], core.State[jobSearchState]] {
	return core.TypedFunc(func(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
		// Build a listing → title lookup for the summary
		titles := make(map[string]string, len(s.Data.Jobs))
		for _, j := range s.Data.Jobs {
			titles[j.ID] = fmt.Sprintf("%s @ %s", j.Title, j.Company)
		}

		var lines []string
		for _, a := range s.Data.Analyses {
			lines = append(lines, fmt.Sprintf("  [fit=%d%%] %s", a.Fit, titles[a.JobID]))
		}
		s.Data.Summary = "Ranked job matches:\n" + strings.Join(lines, "\n")
		return s, nil
	})
}

// generateNode produces a tailored resume and cover letter for each listing.
// Replace the stub with LLM calls that personalise the content per job description.
func generateNode() core.TypedRunnable[core.State[jobSearchState], core.State[jobSearchState]] {
	return core.TypedFunc(func(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
		for _, a := range s.Data.Analyses {
			// ponytail: generate only for jobs with fit ≥ 70
			if a.Fit < 70 {
				continue
			}
			var title string
			for _, j := range s.Data.Jobs {
				if j.ID == a.JobID {
					title = fmt.Sprintf("%s @ %s", j.Title, j.Company)
					break
				}
			}
			s.Data.Docs = append(s.Data.Docs, applicationDocs{
				JobID:       a.JobID,
				Resume:      fmt.Sprintf("[Resume tailored for %s]", title),
				CoverLetter: fmt.Sprintf("[Cover letter tailored for %s]", title),
			})
		}
		return s, nil
	})
}

// submitNode posts each generated application to the appropriate job site.
// Replace the stub with real submission clients per platform.
func submitNode() core.TypedRunnable[core.State[jobSearchState], core.State[jobSearchState]] {
	return core.TypedFunc(func(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
		for _, doc := range s.Data.Docs {
			// ponytail: stub submit — replace with job-board POST calls
			s.Data.Submitted = append(s.Data.Submitted, doc.JobID)
		}
		return s, nil
	})
}

func techAdvisor(j jobListing) advisorScore {
	score := 60
	for _, tag := range j.Tags {
		if tag == "go" || tag == "grpc" || tag == "llm" {
			score += 10
		}
	}
	if score > 100 {
		score = 100
	}
	return advisorScore{Advisor: "tech-advisor", Score: score, Reason: "Go + relevant stack match"}
}

func cultureAdvisor(j jobListing) advisorScore {
	score := 75
	if strings.Contains(strings.ToLower(j.Location), "remote") {
		score = 90
	}
	return advisorScore{Advisor: "culture-advisor", Score: score, Reason: "remote-first alignment"}
}

func compAdvisor(j jobListing) advisorScore {
	// ponytail: no compensation data in stub listings — return neutral score
	return advisorScore{Advisor: "comp-advisor", Score: 70, Reason: "comp data not available in listing"}
}
