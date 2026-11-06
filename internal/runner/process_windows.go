//go:build windows

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x2000
	processAssignAccess               = 0x0100 | 0x0001
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObject          = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	terminateJobObject       = kernel32.NewProc("TerminateJobObject")
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectIOCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimit struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IOInfo                jobObjectIOCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type checkProcess struct {
	job syscall.Handle
}

func startCheckProcess(command *exec.Cmd) (*checkProcess, error) {
	handle, _, callErr := createJobObject.Call(0, 0)
	if handle == 0 {
		return nil, fmt.Errorf("create process job: %w", callErr)
	}
	job := syscall.Handle(handle)
	limits := jobObjectExtendedLimit{}
	limits.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	ok, _, callErr := setInformationJobObject.Call(
		uintptr(job),
		jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		unsafe.Sizeof(limits),
	)
	if ok == 0 {
		_ = syscall.CloseHandle(job)
		return nil, fmt.Errorf("configure process job: %w", callErr)
	}

	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= createSuspended

	if err := command.Start(); err != nil {
		_ = syscall.CloseHandle(job)
		return nil, err
	}

	processHandle, err := syscall.OpenProcess(processAssignAccess, false, uint32(command.Process.Pid))
	if err == nil {
		ok, _, callErr = assignProcessToJobObject.Call(uintptr(job), uintptr(processHandle))
		_ = syscall.CloseHandle(processHandle)
		if ok == 0 {
			err = fmt.Errorf("assign suspended process to job: %w", callErr)
		}
	}
	if err == nil {
		err = resumeProcessMainThread(uint32(command.Process.Pid))
	}
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = syscall.CloseHandle(job)
		return nil, err
	}

	return &checkProcess{job: job}, nil
}

func (process *checkProcess) terminate() {
	_, _, _ = terminateJobObject.Call(uintptr(process.job), 1)
}

func (process *checkProcess) close() {
	_ = syscall.CloseHandle(process.job)
}
