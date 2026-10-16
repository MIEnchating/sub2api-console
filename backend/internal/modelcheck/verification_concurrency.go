package modelcheck

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const verificationProbeConcurrency = 4

// Shared by managed-account and onboarding tasks. Waiting for a slot does not
// consume the individual request timeout or publish a running request.
var verificationRequestSlots = make(chan struct{}, 16)

type verificationJob struct {
	index      int
	challenge  verificationChallenge
	row        VerificationProbe
	phase      string
	dependency string
}

type verificationEvent struct {
	index   int
	row     VerificationProbe
	started bool
	sent    bool
}

func verificationJobs(input targetRequest) ([]verificationJob, error) {
	jobs := []verificationJob{}
	for round := 1; round <= input.Rounds; round++ {
		for _, challenge := range verificationPlannedRound(input.Challenges[round-1], round, input) {
			requestID, err := randomTaskID()
			if err != nil {
				return nil, err
			}
			category := verificationCategory(challenge)
			job := verificationJob{index: len(jobs), challenge: challenge, phase: category,
				row: VerificationProbe{ID: challenge.id, Label: challenge.label, Round: round, RequestID: requestID, Prompt: challenge.prompt, Expected: challenge.expected, Category: category}}
			if category == "fingerprint" {
				job.dependency = strings.TrimSuffix(challenge.id, "_repeat")
			}
			if challenge.behavior {
				job.dependency = "ordered-behavior-sampling"
			}
			if challenge.public != nil {
				job.phase += ":" + challenge.public.Kind
				job.dependency = challenge.public.Cell
				job.row.ReferenceParameters = &PublicReferenceParameters{Temperature: challenge.public.Temperature, Instructions: challenge.public.Instructions, MaxOutputTokens: challenge.public.MaxOutputTokens}
			}
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func runVerificationProbes(ctx context.Context, direct, oauth *http.Client, credential credentialResolution, input targetRequest, report *VerificationReport) error {
	jobs, err := verificationJobs(input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	report.ProbeConcurrency = verificationProbeConcurrency
	completed := make(map[int]VerificationProbe)
	running := make(map[int]VerificationProbe)
	publish := func() {
		// Fresh slices keep previously delivered progress snapshots immutable.
		report.Probes = make([]VerificationProbe, 0, len(completed))
		report.CurrentProbes = make([]VerificationProbe, 0, len(running))
		report.Passed = 0
		for i := range jobs {
			if row, ok := completed[i]; ok {
				report.Probes = append(report.Probes, row)
				if row.Status == "passed" {
					report.Passed++
				}
			}
			if row, ok := running[i]; ok {
				report.CurrentProbes = append(report.CurrentProbes, row)
			}
		}
		report.CurrentProbe = nil
		if len(report.CurrentProbes) > 0 {
			first := report.CurrentProbes[0]
			report.CurrentProbe = &first
		}
		if input.Progress != nil {
			input.Progress(*report)
		}
	}
	// Keep sampling phases separate. The first request of each phase checks
	// protocol/parameter support before opening the remaining concurrency slots.
	for start := 0; start < len(jobs) && ctx.Err() == nil; {
		end := start + 1
		for end < len(jobs) && jobs[end].phase == jobs[start].phase && jobs[end].row.Round == jobs[start].row.Round {
			end++
		}
		runVerificationPhase(ctx, jobs[start:end], func(job verificationJob, events chan<- verificationEvent) {
			select {
			case verificationRequestSlots <- struct{}{}:
			case <-ctx.Done():
				events <- verificationEvent{index: job.index}
				return
			}
			defer func() { <-verificationRequestSlots }()
			if ctx.Err() != nil {
				events <- verificationEvent{index: job.index}
				return
			}
			row := job.row
			row.Status = "running"
			row.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
			events <- verificationEvent{index: job.index, row: row, started: true}
			row = executeVerificationProbe(ctx, direct, oauth, credential, input, job.challenge, row)
			if slices.Contains([]string{"rate_limited", "unauthorized", "unsupported_request"}, row.ErrorCode) {
				cancel()
			}
			events <- verificationEvent{index: job.index, row: row, sent: true}
		}, func(event verificationEvent) {
			if event.started {
				running[event.index] = event.row
			} else {
				delete(running, event.index)
				if event.sent {
					completed[event.index] = event.row
				}
			}
			publish()
		})
		start = end
	}
	publish()
	return nil
}

// Only this coordinator invokes progress. Workers return isolated request
// records, and every launched worker is joined before credentials can be cleaned.
func runVerificationPhase(ctx context.Context, jobs []verificationJob, execute func(verificationJob, chan<- verificationEvent), update func(verificationEvent)) {
	events := make(chan verificationEvent)
	var workers sync.WaitGroup
	defer workers.Wait()
	pending := slices.Clone(jobs)
	busy := map[string]bool{}
	inFlight := map[int]verificationJob{}
	limit := 1
	for len(pending) > 0 || len(inFlight) > 0 {
		for ctx.Err() == nil && len(inFlight) < limit {
			next := slices.IndexFunc(pending, func(job verificationJob) bool { return job.dependency == "" || !busy[job.dependency] })
			if next < 0 {
				break
			}
			job := pending[next]
			pending = slices.Delete(pending, next, next+1)
			inFlight[job.index] = job
			if job.dependency != "" {
				busy[job.dependency] = true
			}
			workers.Add(1)
			go func() { defer workers.Done(); execute(job, events) }()
		}
		if len(inFlight) == 0 {
			break
		}
		event := <-events
		if !event.started {
			job := inFlight[event.index]
			delete(inFlight, event.index)
			delete(busy, job.dependency)
			limit = verificationProbeConcurrency
		}
		update(event)
	}
}
