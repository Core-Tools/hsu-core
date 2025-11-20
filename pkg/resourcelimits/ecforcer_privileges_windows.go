//go:build windows
// +build windows

package resourcelimits

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	TOKEN_QUERY          = 0x0008
	SE_PRIVILEGE_ENABLED = 0x00000002
	TokenPrivileges      = 3
)

type LUID struct {
	LowPart  uint32
	HighPart int32
}

type LUID_AND_ATTRIBUTES struct {
	Luid       LUID
	Attributes uint32
}

type TOKEN_PRIVILEGES struct {
	PrivilegeCount uint32
	Privileges     [1]LUID_AND_ATTRIBUTES // Actually an array of privileges
}

// Helper to open current process token
func openProcessToken() (syscall.Token, error) {
	var token syscall.Token
	var currentProcess syscall.Handle = syscall.Handle(^uintptr(0)) // windows.CurrentProcess() equivalent
	err := syscall.OpenProcessToken(currentProcess, TOKEN_QUERY, &token)
	return token, err
}

// Helper to lookup privilege LUID by name
func lookupPrivilegeValue(name string) (LUID, error) {
	var luid LUID
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return luid, err
	}
	// syscall.LookupPrivilegeValue calls advapi32.dll LookupPrivilegeValueW
	r1, _, e1 := syscall.Syscall(procLookupPrivilegeValueW.Addr(), 3,
		0,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&luid)),
	)
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return luid, err
}

// Helper to get token privileges buffer
func getTokenPrivileges(token syscall.Token) ([]byte, error) {
	var needed uint32
	// First call to get size - expected to fail with ERROR_INSUFFICIENT_BUFFER
	err := syscall.GetTokenInformation(token, TokenPrivileges, nil, 0, &needed)
	if err == nil {
		// Should not succeed with nil buffer, expect ERROR_INSUFFICIENT_BUFFER
		return nil, fmt.Errorf("GetTokenInformation succeeded unexpectedly")
	}
	if err != syscall.ERROR_INSUFFICIENT_BUFFER {
		return nil, err
	}

	buf := make([]byte, needed)
	err = syscall.GetTokenInformation(token, TokenPrivileges, &buf[0], needed, &needed)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// Compares two LUIDs for equality
func luidEqual(a, b LUID) bool {
	return a.LowPart == b.LowPart && a.HighPart == b.HighPart
}

var (
	modadvapi32               = syscall.NewLazyDLL("advapi32.dll")
	procLookupPrivilegeValueW = modadvapi32.NewProc("LookupPrivilegeValueW")
)

func hasPrivilege(privilegeName string) (bool, error) {
	token, err := openProcessToken()
	if err != nil {
		return false, fmt.Errorf("OpenProcessToken: %v", err)
	}
	defer token.Close()

	luid, err := lookupPrivilegeValue(privilegeName)
	if err != nil {
		return false, fmt.Errorf("LookupPrivilegeValue %s: %v", privilegeName, err)
	}

	privsBuf, err := getTokenPrivileges(token)
	if err != nil {
		return false, fmt.Errorf("GetTokenInformation: %v", err)
	}

	// Interpret buffer as TOKEN_PRIVILEGES
	tp := (*TOKEN_PRIVILEGES)(unsafe.Pointer(&privsBuf[0]))
	count := tp.PrivilegeCount

	// The actual privileges start at tp.Privileges[0], but array is declared as size 1,
	// so use slice trick:
	privs := (*[1 << 20]LUID_AND_ATTRIBUTES)(unsafe.Pointer(&tp.Privileges[0]))[:count:count]

	for _, pa := range privs {
		if luidEqual(pa.Luid, luid) {
			if pa.Attributes&SE_PRIVILEGE_ENABLED != 0 {
				return true, nil
			}
			return false, nil
		}
	}

	return false, nil
}

func currentProcessHoldsPrivilegesForMemoryLimits() (bool, error) {
	privilegesToCheck := []string{"SeIncreaseQuotaPrivilege", "SeDebugPrivilege"}

	for _, priv := range privilegesToCheck {
		held, err := hasPrivilege(priv)
		if err != nil {
			return false, fmt.Errorf("error checking privilege %s: %v", priv, err)
		}
		if held {
			return true, nil
		}
	}

	return false, nil
}
