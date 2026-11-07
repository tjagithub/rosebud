//go:build !unix

// Package lock stops two rose commands from writing to one bundle at once.
package lock

// Acquire does not lock on this platform.
func Acquire(string) (func(), error) { return func() {}, nil }
