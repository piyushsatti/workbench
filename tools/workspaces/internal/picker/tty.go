//go:build linux || darwin

package picker

import (
	"os"
	"syscall"
	"unsafe"
)

// IsTTY reports whether stdin and stderr are both terminals. A character
// device test is not enough: /dev/null is one, so "< /dev/null" would pass.
func IsTTY() bool {
	return isTTY(os.Stdin, os.Stderr)
}

func isTTY(in, errOut *os.File) bool {
	return isTerminal(in) && isTerminal(errOut)
}

// isTerminal asks the kernel for the file's termios; only a terminal has one.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
