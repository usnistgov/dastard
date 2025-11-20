package unboundedchan

// UnboundedChannel represents an unbounded queue, but data are entered and removed via channels.
// Beware! You almost certainly want T to be a primitive type; use pointers for large objects.
type UnboundedChannel[T any] struct {
	in    chan T
	out   chan T
	queue []T
}

// NewUnboundedChannel creates and initializes an UnboundedChannel
func NewUnboundedChannel[T any]() *UnboundedChannel[T] {
	uc := &UnboundedChannel[T]{
		in:    make(chan T),
		out:   make(chan T),
		queue: make([]T, 0),
	}
	go uc.run()
	return uc
}

func (uc *UnboundedChannel[T]) run() {
	for {
		if len(uc.queue) == 0 {
			// If queue is empty, only listen for new incoming data
			val, ok := <-uc.in
			if !ok {
				close(uc.out)
				return
			}
			uc.queue = append(uc.queue, val)
		} else {
			// If queue has data, try to send it and also listen for new incoming data
			select {
			case uc.out <- uc.queue[0]:
				uc.queue = uc.queue[1:] // Remove the sent item
			case val, ok := <-uc.in:
				if !ok {
					// When the input channel is closed, send all data currently in the queue, then close the output.
					for _, item := range uc.queue {
						uc.out <- item
					}
					close(uc.out)
					return
				}
				uc.queue = append(uc.queue, val)
			}
		}
	}
}

// In returns the input channel for sending data
func (uc *UnboundedChannel[T]) In() chan<- T {
	return uc.in
}

// Out returns the output channel for receiving data
func (uc *UnboundedChannel[T]) Out() <-chan T {
	return uc.out
}

// Example:
// func main() {
//     unboundedQueue := NewUnboundedChannel[string]()

//     // Goroutine to send data
//     go func() {
//         for i := 0; i < 20; i++ {
//             unboundedQueue.In() <- fmt.Sprintf("unbounded-item-%d", i)
//             fmt.Printf("Sent unbounded-item-%d\n", i)
//             time.Sleep(5 * time.Millisecond)
//         }
//         close(unboundedQueue.In()) // Close the input channel when done
//     }()

//     // Goroutine to receive and process data
//     for d := range unboundedQueue.Out() {
//         fmt.Printf("Processing unbounded data: %s\n", d)
//         time.Sleep(20 * time.Millisecond) // Simulate work
//     }
//     fmt.Println("Unbounded writer goroutine finished.")
// }
