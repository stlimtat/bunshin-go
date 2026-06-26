package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

// jobSearchState is the typed state for the job-search workflow.
// ponytail: duplicated from examples/hello-job-search; both are demos with different entry points
type jobSearchState struct {
	Query     string            `json:"query"`
	Jobs      []jobListing      `json:"jobs"`
	Analyses  []jobAnalysis     `json:"analyses"`
	Summary   string            `json:"summary"`
	Docs      []applicationDocs `json:"docs"`
	Submitted []string          `json:"submitted"`
}

type jobListing struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Company  string   `json:"company"`
	Location string   `json:"location"`
	Tags     []string `json:"tags"`
}

type advisorScore struct {
	Advisor string `json:"advisor"`
	Score   int    `json:"score"`
	Reason  string `json:"reason"`
}

type jobAnalysis struct {
	JobID  string         `json:"job_id"`
	Scores []advisorScore `json:"scores"`
	Fit    int            `json:"fit"`
}

type applicationDocs struct {
	JobID       string `json:"job_id"`
	Resume      string `json:"resume"`
	CoverLetter string `json:"cover_letter"`
}

// newJobSearchRunnable builds the 5-node job-search graph wrapped as a
// core.Runnable that accepts {"query":"..."} JSON and returns typed JSON output.
func newJobSearchRunnable() core.Runnable {
	g := graph.New[jobSearchState]("job-search").
		AddNode(graph.Node[jobSearchState]{
			ID:       "search",
			Runnable: core.TypedFunc(jsSearch),
			Router:   graph.StaticRouter[jobSearchState]("analyze"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "analyze",
			Runnable: core.TypedFunc(jsAnalyze),
			Router:   graph.StaticRouter[jobSearchState]("summarize"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "summarize",
			Runnable: core.TypedFunc(jsSummarize),
			Router:   graph.StaticRouter[jobSearchState]("generate"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "generate",
			Runnable: core.TypedFunc(jsGenerate),
			Router:   graph.StaticRouter[jobSearchState]("submit"),
		}).
		AddNode(graph.Node[jobSearchState]{
			ID:       "submit",
			Runnable: core.TypedFunc(jsSubmit),
		}).
		SetEntry("search")

	return core.NewRunnableFunc("job-search", func(ctx context.Context, input any) (any, error) {
		var state jobSearchState
		if b, err := json.Marshal(input); err == nil {
			_ = json.Unmarshal(b, &state)
		}
		out, err := g.Invoke(ctx, core.NewState(state))
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	})
}

func jsSearch(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
	s.Data.Jobs = []jobListing{
		{ID: "j1", Title: "Senior Go Engineer", Company: "Acme AI", Location: "Remote", Tags: []string{"go", "kubernetes", "llm"}},
		{ID: "j2", Title: "Platform Engineer", Company: "Globex Cloud", Location: "Remote, SG", Tags: []string{"go", "grpc", "devops"}},
		{ID: "j3", Title: "AI Backend Lead", Company: "Initech Labs", Location: "Remote", Tags: []string{"go", "python", "ai"}},
	}
	return s, nil
}

func jsAnalyze(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
	scorers := []func(jobListing) advisorScore{jsTechAdvisor, jsCultureAdvisor, jsCompAdvisor}
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
}

func jsSummarize(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
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
}

func jsGenerate(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
	for _, a := range s.Data.Analyses {
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
}

func jsSubmit(_ context.Context, s core.State[jobSearchState]) (core.State[jobSearchState], error) {
	for _, doc := range s.Data.Docs {
		s.Data.Submitted = append(s.Data.Submitted, doc.JobID)
	}
	return s, nil
}

func jsTechAdvisor(j jobListing) advisorScore {
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

func jsCultureAdvisor(j jobListing) advisorScore {
	score := 75
	if strings.Contains(strings.ToLower(j.Location), "remote") {
		score = 90
	}
	return advisorScore{Advisor: "culture-advisor", Score: score, Reason: "remote-first alignment"}
}

func jsCompAdvisor(_ jobListing) advisorScore {
	return advisorScore{Advisor: "comp-advisor", Score: 70, Reason: "comp data not available in listing"}
}
