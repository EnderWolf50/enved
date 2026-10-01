package winenv

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Registry is the store the programs use; elevate.Registry adds writing through UAC.
var Registry = Store{ReadAll: ReadAll, Apply: Apply, CanWrite: CanWrite}

func location(s Scope) (registry.Key, string) {
	if s == Machine {
		return registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	}
	return registry.CURRENT_USER, `Environment`
}

// ReadAll is every string variable of a scope, sorted by name.
func ReadAll(s Scope) ([]Var, error) {
	root, path := location(s)
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(0)
	if err != nil {
		return nil, err
	}
	var vars []Var
	for _, name := range names {
		data, typ, err := k.GetStringValue(name)
		if err != nil { // not a string (REG_BINARY, ...): not an environment variable
			continue
		}
		vars = append(vars, Var{name, Value{data, typ}})
	}
	Sort(vars)
	return vars, nil
}

// Read is one variable; ok is false when it is not set.
func Read(s Scope, name string) (v Value, ok bool, err error) {
	root, path := location(s)
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return Value{}, false, err
	}
	defer k.Close()
	return get(k, name)
}

func get(k registry.Key, name string) (Value, bool, error) {
	data, typ, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return Value{}, false, nil
	}
	if err != nil {
		return Value{}, false, err
	}
	return Value{data, typ}, true, nil
}

// Apply writes changes: every one is checked against the registry first, and none is
// written if one fails the check. Each old value is saved to BackupDir before it goes, and
// running programs hear about the change once, at the end.
func Apply(changes []Change) error {
	changes = Ordered(changes)
	keys := map[Scope]registry.Key{}
	defer func() {
		for _, k := range keys {
			k.Close()
		}
	}()
	for _, c := range changes {
		if _, open := keys[c.Scope]; open {
			continue
		}
		root, path := location(c.Scope)
		k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return fmt.Errorf("the %s variables: %w", c.Scope, ErrNeedsAdmin)
		}
		if err != nil {
			return err
		}
		keys[c.Scope] = k
	}
	err := Check(changes, func(s Scope, name string) (*Value, error) {
		v, ok, err := get(keys[s], name)
		if !ok || err != nil {
			return nil, err
		}
		return &v, nil
	})
	if err != nil {
		return err
	}
	for _, c := range changes {
		if c.Old != nil {
			if err := backup(c.Scope, c.Name, *c.Old); err != nil {
				return fmt.Errorf("backup failed, %s not written: %w", c.Name, err)
			}
		}
	}
	defer Broadcast()
	for _, c := range changes {
		k := keys[c.Scope]
		switch {
		case c.New == nil:
			err = k.DeleteValue(c.Name)
			if errors.Is(err, registry.ErrNotExist) {
				err = nil
			}
		case c.New.Expands():
			err = k.SetExpandStringValue(c.Name, c.New.Data)
		default:
			err = k.SetStringValue(c.Name, c.New.Data)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", c, err)
		}
	}
	return nil
}

// CanWrite says whether this process may change a scope: the Machine one needs admin.
func CanWrite(s Scope) bool {
	root, path := location(s)
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

// Expand expands %VARS% with this process's environment.
func Expand(s string) string {
	if x, err := registry.ExpandString(s); err == nil {
		return x
	}
	return s
}

// Broadcast sends WM_SETTINGCHANGE("Environment") so Explorer, and whatever it starts next,
// sees the new values without signing out.
func Broadcast() {
	env, _ := windows.UTF16PtrFromString("Environment")
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, 0)
}
