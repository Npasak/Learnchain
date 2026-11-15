//go:build !windows

package main

import "elearning-sbt/handlers"

func StartTrayUI(port string, syncHandler *handlers.SyncHandler, onOpenBrowser func(), onQuit func()) {
	// No-op on non-Windows platforms
}
