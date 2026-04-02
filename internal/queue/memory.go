package queue

type MemoryQueue struct {
	ch chan Job
}

func NewMemoryQueue(bufferSize int) *MemoryQueue {
	return &MemoryQueue{
		ch: make(chan Job, bufferSize),
	}
}

func (q *MemoryQueue) Push(job Job) {
	q.ch <- job
}

func (q *MemoryQueue) Pop() <-chan Job {
	return q.ch
}

func (q *MemoryQueue) Len() int {
	return len(q.ch)
}
