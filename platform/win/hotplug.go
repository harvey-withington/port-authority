//go:build windows

package win

// Hotplug notifications via CM_Register_Notification. Windows calls us on
// a system thread for every USB device / hub interface arrival or removal;
// we translate the symbolic link into the same PnP instance ID the
// topology uses and push a TopologyEvent.

import (
	"context"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"portauthority/core/model"
)

var (
	procCMRegisterNotification   = modcfgmgr32.NewProc("CM_Register_Notification")
	procCMUnregisterNotification = modcfgmgr32.NewProc("CM_Unregister_Notification")
)

// GUID_DEVINTERFACE_USB_DEVICE {A5DCBF10-6530-11D2-901F-00C04FB951ED}
var guidUSBDevice = windows.GUID{
	Data1: 0xA5DCBF10, Data2: 0x6530, Data3: 0x11D2,
	Data4: [8]byte{0x90, 0x1F, 0x00, 0xC0, 0x4F, 0xB9, 0x51, 0xED},
}

// GUID_DEVINTERFACE_USB_HUB {F18A0E88-C30C-11D0-8815-00A0C906BED8}
var guidUSBHub = windows.GUID{
	Data1: 0xF18A0E88, Data2: 0xC30C, Data3: 0x11D0,
	Data4: [8]byte{0x88, 0x15, 0x00, 0xA0, 0xC9, 0x06, 0xBE, 0xD8},
}

const (
	cmNotifyFilterTypeDeviceInterface    = 0
	cmNotifyActionDeviceInterfaceArrival = 0
	cmNotifyActionDeviceInterfaceRemoval = 1
	maxDeviceIDLen                       = 200
)

// cmNotifyFilter mirrors CM_NOTIFY_FILTER: a 16-byte header followed by a
// union whose largest member is WCHAR InstanceId[MAX_DEVICE_ID_LEN].
type cmNotifyFilter struct {
	CbSize     uint32
	Flags      uint32
	FilterType uint32
	Reserved   uint32
	ClassGUID  windows.GUID
	_          [maxDeviceIDLen*2 - 16]byte
}

// CM_NOTIFY_EVENT_DATA for device interfaces:
//
//	0 DWORD FilterType, 4 DWORD Reserved, 8 GUID ClassGuid, 24 WCHAR SymbolicLink[]
const cmEventDataSymbolicLinkOffset = 24

var (
	hotplugMu        sync.Mutex
	hotplugWatchers          = map[uintptr]*hotplugWatcher{}
	hotplugNextID    uintptr = 1
	hotplugCallback  uintptr
	hotplugCallbackO sync.Once
)

type hotplugWatcher struct {
	ch      chan model.TopologyEvent
	onEvent func(model.TopologyEvent)
}

func hotplugNotifyCallback(hNotify, context uintptr, action uint32, data *byte, size uint32) uintptr {
	hotplugMu.Lock()
	w := hotplugWatchers[context]
	hotplugMu.Unlock()
	if w == nil || data == nil || size < cmEventDataSymbolicLinkOffset+2 {
		return 0
	}
	defer func() { _ = recover() }()

	raw := unsafe.Slice(data, size)
	link := decodeWide(raw[cmEventDataSymbolicLinkOffset:])
	ev := model.TopologyEvent{At: time.Now(), Detail: link, DeviceID: canonicalInstanceID(instanceIDFromInterface(link))}
	_ = hNotify
	switch action {
	case cmNotifyActionDeviceInterfaceArrival:
		ev.Kind = model.EventDeviceAdded
	case cmNotifyActionDeviceInterfaceRemoval:
		ev.Kind = model.EventDeviceRemoved
	default:
		return 0
	}
	if w.onEvent != nil {
		w.onEvent(ev)
	}
	select {
	case w.ch <- ev:
	default:
		// A consumer that is not draining loses events rather than
		// stalling the PnP manager's thread.
	}
	return 0
}

// canonicalInstanceID upper-cases an instance ID; Windows treats them as
// case-insensitive and SetupAPI reports them upper-cased.
func canonicalInstanceID(id string) string {
	return strings.ToUpper(id)
}

// Watch streams USB hotplug events until ctx is cancelled.
func (p *Provider) Watch(ctx context.Context) (<-chan model.TopologyEvent, error) {
	hotplugCallbackO.Do(func() {
		hotplugCallback = windows.NewCallback(hotplugNotifyCallback)
	})
	w := &hotplugWatcher{ch: make(chan model.TopologyEvent, 64), onEvent: p.onHotplug}

	hotplugMu.Lock()
	id := hotplugNextID
	hotplugNextID++
	hotplugWatchers[id] = w
	hotplugMu.Unlock()

	var handles []uintptr
	for _, guid := range []windows.GUID{guidUSBDevice, guidUSBHub} {
		filter := &cmNotifyFilter{FilterType: cmNotifyFilterTypeDeviceInterface, ClassGUID: guid}
		filter.CbSize = uint32(unsafe.Sizeof(*filter))
		var h uintptr
		ret, _, _ := procCMRegisterNotification.Call(
			uintptr(unsafe.Pointer(filter)), id, hotplugCallback, uintptr(unsafe.Pointer(&h)))
		if ret != 0 {
			for _, done := range handles {
				procCMUnregisterNotification.Call(done)
			}
			hotplugMu.Lock()
			delete(hotplugWatchers, id)
			hotplugMu.Unlock()
			return nil, &windowsError{op: "CM_Register_Notification", cr: uint32(ret)}
		}
		handles = append(handles, h)
	}

	go func() {
		<-ctx.Done()
		for _, h := range handles {
			procCMUnregisterNotification.Call(h) // blocks until in-flight callbacks return
		}
		hotplugMu.Lock()
		delete(hotplugWatchers, id)
		hotplugMu.Unlock()
		close(w.ch)
	}()
	return w.ch, nil
}

type windowsError struct {
	op string
	cr uint32
}

func (e *windowsError) Error() string {
	return e.op + ": CONFIGRET 0x" + strings.ToUpper(strconvHex(e.cr))
}

func strconvHex(v uint32) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v&0xf]
		v >>= 4
	}
	return string(b[i:])
}
