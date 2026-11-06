//go:build windows

package runner

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	createSuspended        = 0x00000004
	snapshotThreads        = 0x00000004
	threadSuspendResume    = 0x0002
)

var (
	createThreadSnapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	thread32First        = kernel32.NewProc("Thread32First")
	thread32Next         = kernel32.NewProc("Thread32Next")
	openThread           = kernel32.NewProc("OpenThread")
	resumeThread         = kernel32.NewProc("ResumeThread")
)

type threadEntry32 struct {
	size           uint32
	usage          uint32
	threadID       uint32
	ownerProcessID uint32
	basePriority   int32
	deltaPriority  int32
	flags          uint32
}

func resumeProcessMainThread(pid uint32) error {
	snapshot, _, callErr := createThreadSnapshot.Call(snapshotThreads, 0)
	if snapshot == ^uintptr(0) {
		return fmt.Errorf("snapshot process threads: %w", callErr)
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))

	entry := threadEntry32{size: uint32(unsafe.Sizeof(threadEntry32{}))}
	ok, _, callErr := thread32First.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return fmt.Errorf("enumerate process threads: %w", callErr)
	}
	for {
		if entry.ownerProcessID == pid {
			handle, _, callErr := openThread.Call(threadSuspendResume, 0, uintptr(entry.threadID))
			if handle == 0 {
				return fmt.Errorf("open suspended process thread: %w", callErr)
			}
			defer syscall.CloseHandle(syscall.Handle(handle))

			previousCount, _, callErr := resumeThread.Call(handle)
			if previousCount == ^uintptr(0) {
				return fmt.Errorf("resume process thread: %w", callErr)
			}
			if previousCount != 1 {
				return fmt.Errorf("resume process thread: unexpected suspend count %d", previousCount)
			}
			return nil
		}

		ok, _, _ = thread32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ok == 0 {
			return fmt.Errorf("suspended process %d has no enumerated thread", pid)
		}
	}
}
