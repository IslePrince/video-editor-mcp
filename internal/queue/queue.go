package queue

type Job struct {
	ID        string
	ProjectID string
	Execute   func() error
}

type Queue interface {
	Push(job Job)
	Pop() <-chan Job
	Len() int
}
