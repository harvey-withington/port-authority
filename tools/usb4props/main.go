//go:build windows

// usb4props dumps every device property and every "Device
// Parameters" registry value of each USB4 devnode, to find where Windows
// keeps things like the negotiated USB4 link rate. A maintainer tool for
// checking the undocumented router property set on a new machine or
// driver build; see platform/win/usb4.go for what each key holds.
//
//	go run ./tools/usb4props
package main

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modcfgmgr32                = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMGetDevNodePropKeys   = modcfgmgr32.NewProc("CM_Get_DevNode_Property_Keys")
	procCMGetDevNodePropertyW  = modcfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
)

type devPropKey struct {
	fmtid windows.GUID
	pid   uint32
}

func main() {
	set, err := windows.SetupDiGetClassDevsEx(nil, "USB4", 0, windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		panic(err)
	}
	defer set.Close()
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if err != nil {
			break
		}
		id, _ := set.DeviceInstanceID(data)
		fmt.Printf("\n===== %s\n", id)
		dumpProps(uint32(data.DevInst))
		dumpRegistry(id)
	}
}

func dumpProps(devInst uint32) {
	var count uint32
	procCMGetDevNodePropKeys.Call(uintptr(devInst), 0, uintptr(unsafe.Pointer(&count)), 0)
	if count == 0 {
		fmt.Println("  (no property keys)")
		return
	}
	keys := make([]devPropKey, count)
	ret, _, _ := procCMGetDevNodePropKeys.Call(uintptr(devInst), uintptr(unsafe.Pointer(&keys[0])), uintptr(unsafe.Pointer(&count)), 0)
	if ret != 0 {
		fmt.Printf("  property keys: CR %d\n", ret)
		return
	}
	for _, k := range keys[:count] {
		var typ uint32
		var size uint32
		procCMGetDevNodePropertyW.Call(uintptr(devInst), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)), 0)
		buf := make([]byte, size+2)
		procCMGetDevNodePropertyW.Call(uintptr(devInst), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
		fmt.Printf("  {%s},%d type=0x%x: %s\n", k.fmtid.String(), k.pid, typ, render(typ, buf[:size]))
	}
}

func render(typ uint32, b []byte) string {
	switch typ & 0xFFF {
	case 0x12, 0x2012: // STRING, STRING_LIST
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
		s := windows.UTF16ToString(u)
		if typ&0x2000 != 0 {
			parts := strings.Split(strings.TrimRight(string(utf16Join(u)), "\x00"), "\x00")
			return fmt.Sprintf("%q", parts)
		}
		return fmt.Sprintf("%q", s)
	case 0x07: // UINT32
		if len(b) >= 4 {
			return fmt.Sprintf("%d (0x%x)", binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b))
		}
	case 0x08: // UINT64
		if len(b) >= 8 {
			return fmt.Sprintf("%d", binary.LittleEndian.Uint64(b))
		}
	case 0x11: // BOOLEAN
		if len(b) >= 1 {
			return fmt.Sprintf("%v", b[0] != 0)
		}
	case 0x13: // GUID
		if len(b) >= 16 {
			return (*windows.GUID)(unsafe.Pointer(&b[0])).String()
		}
	case 0x10: // FILETIME
		return fmt.Sprintf("filetime % x", b)
	}
	if len(b) > 64 {
		return fmt.Sprintf("% x ... (%d bytes)", b[:64], len(b))
	}
	return fmt.Sprintf("% x", b)
}

func utf16Join(u []uint16) []byte {
	var sb strings.Builder
	for _, c := range u {
		if c < 0x80 {
			sb.WriteByte(byte(c))
		} else {
			sb.WriteRune(rune(c))
		}
	}
	return []byte(sb.String())
}

func dumpRegistry(instanceID string) {
	path := `SYSTEM\CurrentControlSet\Enum\` + instanceID + `\Device Parameters`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
	if err != nil {
		fmt.Printf("  registry: %v\n", err)
		return
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	for _, n := range names {
		if s, _, err := k.GetStringValue(n); err == nil {
			fmt.Printf("  reg %s = %q\n", n, s)
		} else if v, _, err := k.GetIntegerValue(n); err == nil {
			fmt.Printf("  reg %s = %d\n", n, v)
		} else if b, _, err := k.GetBinaryValue(n); err == nil {
			fmt.Printf("  reg %s = % x\n", n, b)
		}
	}
	subs, _ := k.ReadSubKeyNames(-1)
	if len(subs) > 0 {
		fmt.Printf("  reg subkeys: %v\n", subs)
	}
}
