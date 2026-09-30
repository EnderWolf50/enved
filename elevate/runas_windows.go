package elevate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	hwnd          windows.Handle
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}

// run starts this program elevated on one argument of changes and waits for it.
func run(arg string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	// The elevated copy leaves the reason for a failure here; only a message for the user,
	// so it may live in a file.
	result := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d-%d.txt", name, os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(result)

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(fmt.Sprintf(`%s %s "%s"`, Flag, arg, result))
	const seeMaskNoCloseProcess, swHide = 0x40, 0
	info := shellExecuteInfo{mask: seeMaskNoCloseProcess, verb: verb, file: file, parameters: params, show: swHide}
	info.size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	if ok, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ErrDeclined
		}
		return fmt.Errorf("could not start an elevated %s: %w", name, callErr)
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		msg, _ := os.ReadFile(result)
		if len(msg) == 0 {
			msg = []byte(fmt.Sprintf("the elevated %s exited with %d", name, code))
		}
		return errors.New(strings.TrimSpace(string(msg)))
	}
	return nil
}
