package main

import (
	"fmt"
	"sync"
	"time"
)

// Spinner represents a loading animation
type Spinner struct {
	frames   []string
	interval time.Duration
	message  string
	stop     chan bool
	wg       sync.WaitGroup
	active   bool
	mu       sync.Mutex
}

// NewSpinner creates a new spinner with aquamarine dots
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:   []string{".", "·", "•", "¤", "°", "¤", "•", "·"},
		interval: 100 * time.Millisecond,
		message:  message,
		stop:     make(chan bool),
		active:   false,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		i := 0
		for {
			select {
			case <-s.stop:
				// Clear the line
				fmt.Printf("\r\033[K")
				return
			default:
				// Aquamarine color: RGB(127, 255, 212) = \033[38;2;127;255;212m
				fmt.Printf("\r\033[38;2;127;255;212m%s %s\033[0m", s.message, s.frames[i%len(s.frames)])
				i++
				time.Sleep(s.interval)
			}
		}
	}()
}

// Stop ends the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	s.stop <- true
	s.wg.Wait()
}

// UpdateMessage changes the spinner message while running
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}
