// Package async provides bounded, in-memory asynchronous execution for a
// gobus.Bus. A Runtime owns fixed worker pools and must be started and shut
// down explicitly.
//
// By default, a submission context controls both queue admission and handler
// execution. WithExecutionContext separates those lifetimes for background
// work. Runtime shutdown cancellation still reaches active jobs regardless of
// the selected execution context.
//
// Jobs are attempted at most once and are not persisted. Process termination
// loses queued work.
package async
