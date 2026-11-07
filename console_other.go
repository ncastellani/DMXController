//go:build !windows

package main

// waitBeforeClosing só faz algo no Windows (ver console_windows.go).
func waitBeforeClosing() {}
