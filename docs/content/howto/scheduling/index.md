+++
title = 'Scheduling Workflow Invocations'
date = '2026-06-05'
draft = false
weight = 5
toc = true
+++

# Scheduling Workflow Invocations

bunshin-go workflows are triggered by calling `Invoke` or `Stream` on a `Graph[S]` or `Chain[S]`. Scheduling is external — any mechanism that calls that function on a schedule works.

---

## Simple in-process cron

Use `robfig/cron` for recurring invocations within a long-running process:

```go
import "github.com/robfig/cron/v3"

c := cron.New()
c.AddFunc("0 * * * *", func() { // every hour
    result, err := agent.Invoke(context.Background(), core.NewState(MyState{
        Task: "generate daily summary",
    }))
    if err != nil {
        log.Error().Err(err).Msg("scheduled run failed")
    }
    _ = result
})
c.Start()
defer c.Stop()
```

---

## CLI one-shot invocation

The bunshin CLI supports one-shot invocation suitable for OS cron or CI pipelines:

```bash
# OS cron: run every day at 02:00
0 2 * * * /usr/local/bin/bunshin agent --question "Summarise yesterday's events"
```

The CLI exits with code 0 on success, non-zero on error, making it composable with standard job schedulers.

---

## Kubernetes CronJob

For horizontally scaled workers that spin up on demand:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: bunshin-daily-summary
spec:
  schedule: "0 2 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
          - name: bunshin
            image: ghcr.io/stlimtat/bunshin-go:latest
            command: ["bunshin", "agent"]
            args: ["--question", "Summarise yesterday's events"]
            env:
            - name: BUNSHIN_PROVIDER
              value: anthropic
            - name: BUNSHIN_API_KEY
              valueFrom:
                secretKeyRef:
                  name: bunshin-secrets
                  key: anthropic-api-key
            - name: BUNSHIN_CHECKPOINT_BACKEND
              value: postgres
            - name: BUNSHIN_CHECKPOINT_DSN
              valueFrom:
                secretKeyRef:
                  name: bunshin-secrets
                  key: postgres-dsn
```

Each pod run is stateless: the workflow state is persisted in the Checkpoint backend (Postgres) and resumed if the pod is restarted via `ThreadID`.

---

## Kubernetes worker pool

For task-queue style execution where multiple workers process jobs concurrently:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bunshin-worker
spec:
  replicas: 4
  selector:
    matchLabels:
      app: bunshin-worker
  template:
    spec:
      containers:
      - name: worker
        image: ghcr.io/stlimtat/bunshin-go:latest
        command: ["bunshin", "serve", "--mode=worker"]
        env:
        - name: BUNSHIN_QUEUE_BACKEND
          value: redis
        - name: BUNSHIN_REDIS_URL
          valueFrom:
            secretKeyRef:
              name: bunshin-secrets
              key: redis-url
```

Workers pick jobs from a Redis queue (planned: `pkg/transport` queue backend). `ThreadID` ensures that if a worker pod is evicted mid-workflow, another worker resumes from the last checkpoint rather than starting over.

---

## ThreadID and horizontal scaling

When scheduling across multiple workers, set `ThreadID` explicitly so checkpoints can be resumed regardless of which pod runs the job:

```go
state := core.NewState(MyState{Task: "process-batch-2026-06-05"}).
    WithMeta(core.MetaThreadID, "daily-summary-2026-06-05")

result, err := agent.Invoke(ctx, state)
```

Two workers with the same `ThreadID` will not run simultaneously if the Checkpoint backend uses optimistic locking (Postgres, Redis with SETNX).

---

## Temporal / Dagger integration (planned)

bunshin-go workflows are plain Go functions, so Temporal activities and workflows can call them directly:

```go
// Temporal activity wrapping a bunshin workflow
func RunSummaryWorkflow(ctx context.Context, date string) (string, error) {
    result, err := agent.Invoke(ctx, core.NewState(SummaryState{Date: date}))
    if err != nil {
        return "", err
    }
    return result.Data.Summary, nil
}
```

Temporal provides durable execution guarantees on top of bunshin's checkpoint/resume model.
