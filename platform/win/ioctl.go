//go:build windows

package win

// Raw USB hub / host-controller IOCTL plumbing.
//
// Struct layouts are hand-decoded by byte offset because usbioctl.h wraps
// these structures in #pragma pack(1); Go structs would need manual
// padding control anyway. Offsets are documented next to each parser and
// were verified against Windows SDK 10.0.26100 usbioctl.h.

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CTL_CODE(FILE_DEVICE_USB=0x22, function, METHOD_BUFFERED, FILE_ANY_ACCESS)
// = 0x220000 | function<<2.
const (
	ioctlGetHCDDriverKeyName                 = 0x220424 // HCD_GET_DRIVERKEY_NAME 265
	ioctlUSBGetRootHubName                   = 0x220408 // HCD_GET_ROOT_HUB_NAME 258
	ioctlUSBGetNodeInformation               = 0x220408 // USB_GET_NODE_INFORMATION 258
	ioctlUSBGetDescriptorFromNodeConnection  = 0x220410 // 260
	ioctlUSBGetNodeConnectionName            = 0x220414 // 261
	ioctlUSBGetNodeConnectionDriverKeyName   = 0x220420 // 264
	ioctlUSBGetNodeConnectionInformationEx   = 0x220448 // 274
	ioctlUSBGetHubCapabilitiesEx             = 0x220450 // 276
	ioctlUSBGetHubInformationEx              = 0x220454 // 277
	ioctlUSBGetPortConnectorProperties       = 0x220458 // 278
	ioctlUSBGetNodeConnectionInformationExV2 = 0x22045C // 279
)

// GUID_DEVINTERFACE_USB_HOST_CONTROLLER {3ABF6F2D-71C4-462a-8A92-1E6861E6AF27}
var guidUSBHostController = windows.GUID{
	Data1: 0x3abf6f2d, Data2: 0x71c4, Data3: 0x462a,
	Data4: [8]byte{0x8a, 0x92, 0x1e, 0x68, 0x61, 0xe6, 0xaf, 0x27},
}

var (
	modcfgmgr32                       = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMGetDeviceInterfaceListSizeW = modcfgmgr32.NewProc("CM_Get_Device_Interface_List_SizeW")
	procCMGetDeviceInterfaceListW     = modcfgmgr32.NewProc("CM_Get_Device_Interface_ListW")
)

const cmGetDeviceInterfaceListPresent = 0

// deviceInterfaces lists the symbolic links of every present device
// interface of the given class.
func deviceInterfaces(class *windows.GUID) ([]string, error) {
	var size uint32
	ret, _, _ := procCMGetDeviceInterfaceListSizeW.Call(
		uintptr(unsafe.Pointer(&size)),
		uintptr(unsafe.Pointer(class)),
		0,
		cmGetDeviceInterfaceListPresent,
	)
	if ret != 0 {
		return nil, fmt.Errorf("CM_Get_Device_Interface_List_SizeW: CR 0x%x", ret)
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]uint16, size)
	ret, _, _ = procCMGetDeviceInterfaceListW.Call(
		uintptr(unsafe.Pointer(class)),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(size),
		cmGetDeviceInterfaceListPresent,
	)
	if ret != 0 {
		return nil, fmt.Errorf("CM_Get_Device_Interface_ListW: CR 0x%x", ret)
	}
	// The buffer is a double-NUL-terminated list of NUL-separated strings.
	var out []string
	start := 0
	for i, c := range buf {
		if c == 0 {
			if i > start {
				out = append(out, string(utf16.Decode(buf[start:i])))
			}
			start = i + 1
		}
	}
	return out, nil
}

// openDevice opens a hub or controller symbolic link for IOCTL use, the
// same way USBView does.
func openDevice(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	return h, nil
}

// ioctl issues a buffered DeviceIoControl and returns the bytes written to
// out. in and out may alias the same slice.
func ioctl(h windows.Handle, code uint32, in, out []byte) (uint32, error) {
	var inPtr, outPtr *byte
	if len(in) > 0 {
		inPtr = &in[0]
	}
	if len(out) > 0 {
		outPtr = &out[0]
	}
	var n uint32
	err := windows.DeviceIoControl(h, code, inPtr, uint32(len(in)), outPtr, uint32(len(out)), &n, nil)
	if err != nil {
		return n, fmt.Errorf("ioctl 0x%x: %w", code, err)
	}
	return n, nil
}

// readNameNoIndex handles USB_ROOT_HUB_NAME / USB_HCD_DRIVERKEY_NAME:
//
//	ULONG ActualLength; WCHAR Name[]
func readNameNoIndex(h windows.Handle, code uint32) (string, error) {
	probe := make([]byte, 8)
	if _, err := ioctl(h, code, probe, probe); err != nil {
		return "", err
	}
	total := binary.LittleEndian.Uint32(probe[0:])
	if total < 8 || total > 1<<16 {
		return "", fmt.Errorf("ioctl 0x%x: bad ActualLength %d", code, total)
	}
	buf := make([]byte, total)
	n, err := ioctl(h, code, buf, buf)
	if err != nil {
		return "", err
	}
	return decodeWide(buf[4:n]), nil
}

// readNameWithIndex handles USB_NODE_CONNECTION_NAME /
// USB_NODE_CONNECTION_DRIVERKEY_NAME:
//
//	ULONG ConnectionIndex; ULONG ActualLength; WCHAR Name[]
func readNameWithIndex(h windows.Handle, code uint32, index uint32) (string, error) {
	probe := make([]byte, 12)
	binary.LittleEndian.PutUint32(probe[0:], index)
	if _, err := ioctl(h, code, probe, probe); err != nil {
		return "", err
	}
	total := binary.LittleEndian.Uint32(probe[4:])
	if total < 12 || total > 1<<16 {
		return "", fmt.Errorf("ioctl 0x%x: bad ActualLength %d", code, total)
	}
	buf := make([]byte, total)
	binary.LittleEndian.PutUint32(buf[0:], index)
	n, err := ioctl(h, code, buf, buf)
	if err != nil {
		return "", err
	}
	return decodeWide(buf[8:n]), nil
}

// decodeWide converts a NUL-terminated UTF-16LE byte slice to a string.
func decodeWide(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

// getDescriptor issues IOCTL_USB_GET_DESCRIPTOR_FROM_NODE_CONNECTION.
//
// USB_DESCRIPTOR_REQUEST (packed, 12-byte header):
//
//	 0 ULONG  ConnectionIndex
//	 4 UCHAR  bmRequest   (0x80 device-to-host)
//	 5 UCHAR  bRequest    (6 GET_DESCRIPTOR)
//	 6 USHORT wValue      (type<<8 | index)
//	 8 USHORT wIndex      (language id for strings)
//	10 USHORT wLength
//	12 UCHAR  Data[]
func getDescriptor(h windows.Handle, index uint32, descType, descIndex uint8, langID uint16, length uint16) ([]byte, error) {
	buf := make([]byte, 12+int(length))
	binary.LittleEndian.PutUint32(buf[0:], index)
	buf[4] = 0x80
	buf[5] = 6
	binary.LittleEndian.PutUint16(buf[6:], uint16(descType)<<8|uint16(descIndex))
	binary.LittleEndian.PutUint16(buf[8:], langID)
	binary.LittleEndian.PutUint16(buf[10:], length)
	n, err := ioctl(h, ioctlUSBGetDescriptorFromNodeConnection, buf, buf)
	if err != nil {
		return nil, err
	}
	if n < 12 {
		return nil, fmt.Errorf("descriptor %d/%d: short reply (%d bytes)", descType, descIndex, n)
	}
	return buf[12:n], nil
}

const (
	descTypeString = 3
	descTypeConfig = 2
)

// stringDescriptor reads one string descriptor, trying the device's
// first supported language and falling back to US English.
func stringDescriptor(h windows.Handle, index uint32, strIndex uint8, langID uint16) (string, error) {
	data, err := getDescriptor(h, index, descTypeString, strIndex, langID, 255)
	if err != nil {
		return "", err
	}
	if len(data) < 2 || data[1] != descTypeString {
		return "", fmt.Errorf("string descriptor %d: malformed", strIndex)
	}
	end := int(data[0])
	if end > len(data) {
		end = len(data)
	}
	return strings.TrimSpace(decodeWide(data[2:end])), nil
}

// firstLanguage returns the device's first supported string language id,
// or 0x0409 (en-US) when the query fails.
func firstLanguage(h windows.Handle, index uint32) uint16 {
	data, err := getDescriptor(h, index, descTypeString, 0, 0, 255)
	if err == nil && len(data) >= 4 && data[1] == descTypeString {
		if lang := binary.LittleEndian.Uint16(data[2:]); lang != 0 {
			return lang
		}
	}
	return 0x0409
}

// configDescriptor reads the full active configuration descriptor
// (header + interfaces + endpoints).
func configDescriptor(h windows.Handle, index uint32) ([]byte, error) {
	head, err := getDescriptor(h, index, descTypeConfig, 0, 0, 9)
	if err != nil {
		return nil, err
	}
	if len(head) < 4 {
		return nil, fmt.Errorf("config descriptor: short header")
	}
	total := binary.LittleEndian.Uint16(head[2:])
	if total < 9 {
		return nil, fmt.Errorf("config descriptor: bad wTotalLength %d", total)
	}
	return getDescriptor(h, index, descTypeConfig, 0, 0, total)
}
