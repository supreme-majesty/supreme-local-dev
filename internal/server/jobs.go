package server

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// JobStatus is the state of a background operation.
type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

const maxJobOutput = 512 << 10 // keep the last 512 KiB of output per job

// Job is a long-running operation (start, stop, setup...) whose output the
// dashboard follows live.
type Job struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ProjectID string    `json:"project_id,omitempty"`
	Status    JobStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`

	mu      sync.Mutex
	output  []byte
	updated chan struct{} // closed and replaced on every write/finish
}

// Write appends output (implements io.Writer for the runner).
func (j *Job) Write(p []byte) (int, error) {
	j.mu.Lock()
	j.output = append(j.output, p...)
	if len(j.output) > maxJobOutput {
		j.output = j.output[len(j.output)-maxJobOutput:]
	}
	j.notifyLocked()
	j.mu.Unlock()
	return len(p), nil
}

func (j *Job) notifyLocked() {
	close(j.updated)
	j.updated = make(chan struct{})
}

// Snapshot returns the job's state and output from offset onwards, plus a
// channel that is closed when more output arrives or the job finishes.
func (j *Job) Snapshot(offset int) (JobView, <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if offset < 0 || offset > len(j.output) {
		offset = 0
	}
	return JobView{
		ID: j.ID, Kind: j.Kind, ProjectID: j.ProjectID, Status: j.Status, Error: j.Error,
		StartedAt: j.StartedAt, EndedAt: j.EndedAt,
		Output: string(j.output[offset:]), Offset: len(j.output),
	}, j.updated
}

// JobView is the JSON form of a job.
type JobView struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ProjectID string    `json:"project_id,omitempty"`
	Status    JobStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	Output    string    `json:"output"`
	Offset    int       `json:"offset"` // pass back as ?offset= to receive only new output
}

// Jobs runs background operations, at most one per project at a time.
type Jobs struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	order   []string
	busy    map[string]string // project id -> running job id
	keepMax int
}

// NewJobs creates an empty job registry.
func NewJobs() *Jobs {
	return &Jobs{jobs: map[string]*Job{}, busy: map[string]string{}, keepMax: 100}
}

// ErrBusy is returned when a project already has a running job.
type ErrBusy struct{ JobID string }

func (e ErrBusy) Error() string {
	return "another operation is running for this project (job " + e.JobID + ")"
}

// Start runs fn in the background and returns its job. fn writes progress to the job.
func (js *Jobs) Start(kind, projectID string, fn func(ctx context.Context, job *Job) error) (*Job, error) {
	js.mu.Lock()
	if projectID != "" {
		if running, ok := js.busy[projectID]; ok {
			js.mu.Unlock()
			return nil, ErrBusy{JobID: running}
		}
	}
	id, err := randomString(9)
	if err != nil {
		js.mu.Unlock()
		return nil, err
	}
	job := &Job{ID: id, Kind: kind, ProjectID: projectID, Status: JobRunning, StartedAt: time.Now().UTC(), updated: make(chan struct{})}
	js.jobs[id] = job
	js.order = append(js.order, id)
	if projectID != "" {
		js.busy[projectID] = id
	}
	js.pruneLocked()
	js.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		var runErr error
		func() {
			defer func() {
				if p := recover(); p != nil {
					runErr = fmt.Errorf("internal error: %v", p)
				}
			}()
			runErr = fn(ctx, job)
		}()

		job.mu.Lock()
		job.EndedAt = time.Now().UTC()
		if runErr != nil {
			job.Status, job.Error = JobFailed, runErr.Error()
		} else {
			job.Status = JobSucceeded
		}
		job.notifyLocked()
		job.mu.Unlock()

		js.mu.Lock()
		if js.busy[projectID] == id {
			delete(js.busy, projectID)
		}
		js.mu.Unlock()
	}()
	return job, nil
}

// Get returns a job by id.
func (js *Jobs) Get(id string) (*Job, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	j, ok := js.jobs[id]
	return j, ok
}

// Running returns the running job of a project, if any.
func (js *Jobs) Running(projectID string) string {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.busy[projectID]
}

func (js *Jobs) pruneLocked() {
	for len(js.order) > js.keepMax {
		oldest := js.order[0]
		if j := js.jobs[oldest]; j != nil {
			j.mu.Lock()
			running := j.Status == JobRunning
			j.mu.Unlock()
			if running {
				return
			}
		}
		delete(js.jobs, oldest)
		js.order = js.order[1:]
	}
}
