package view

// task is a background job (walk, search) whose results are applied on the
// viewer's loop: the goroutine sends closures to bg, and stops sending once
// the task is cancelled.
type task struct{ stop chan struct{} }

func newTask() *task { return &task{stop: make(chan struct{})} }

func (t *task) cancel() {
	if t != nil {
		select {
		case <-t.stop:
		default:
			close(t.stop)
		}
	}
}

func (t *task) stopped() bool {
	select {
	case <-t.stop:
		return true
	default:
		return false
	}
}

// send hands f to the loop; false when the task was cancelled meanwhile.
func (t *task) send(bg chan<- func(*model), f func(*model)) bool {
	select {
	case bg <- f:
		return true
	case <-t.stop:
		return false
	}
}

// applyBackground applies f, a result of background work, and what else is
// ready (a few), so the loop draws once for all of them.
func (m *model) applyBackground(f func(*model)) {
	f(m)
	for range 64 {
		select {
		case f := <-m.bg:
			f(m)
		default:
			return
		}
	}
}
