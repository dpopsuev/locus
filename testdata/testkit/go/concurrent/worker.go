// Package concurrent provides fixtures for async/function-colour edge detection.
package concurrent

import "sync"

// Produce sends values onto ch — emits channel_send edge to ch.
func Produce(ch chan<- int, n int) {
	for i := range n {
		ch <- i // channel_send
	}
	close(ch)
}

// Consume reads from ch — emits channel_recv edge to ch.
func Consume(ch <-chan int) []int {
	var out []int
	for v := range ch {
		out = append(out, v)
	}
	return out
}

// Pipeline spawns Produce in a goroutine and consumes results.
// Emits: goroutine edge to Produce, channel_send/recv via ch.
func Pipeline(n int) []int {
	ch := make(chan int, n)
	go Produce(ch, n) // goroutine
	return Consume(ch)
}

// FanOut spawns n workers — emits n goroutine edges to worker.
func FanOut(n int, work func(int)) {
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(id int) { // goroutine
			defer wg.Done()
			work(id)
		}(i)
	}
	wg.Wait()
}
