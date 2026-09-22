//go:build windows

package cli

// pauseForks does nothing on Windows, which has no fork: CreateProcess builds
// the child's handle table explicitly, so it cannot copy a write handle to a
// file this process is in the middle of writing. There is no ETXTBSY race to
// close, and syscall.ForkLock does not exist here.
func pauseForks() (resume func()) { return func() {} }
