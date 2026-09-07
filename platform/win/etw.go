//go:build windows

package win

// Minimal Event Tracing for Windows (ETW) real-time consumer.
//
// Written against advapi32 / tdh directly rather than pulling in a
// library: the mature pure-Go options are GPL-3, and the surface we need
// is small (one session, a few providers, generic property decoding).
//
// Requires administrator rights to start a session. Struct layouts match
// the 64-bit Windows SDK headers; Go's alignment rules produce the same
// offsets as MSVC for these field types, so the structs are declared
// rather than hand-packed.

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modadvapi32                = windows.NewLazySystemDLL("advapi32.dll")
	procStartTraceW            = modadvapi32.NewProc("StartTraceW")
	procControlTraceW          = modadvapi32.NewProc("ControlTraceW")
	procEnableTraceEx2         = modadvapi32.NewProc("EnableTraceEx2")
	procOpenTraceW             = modadvapi32.NewProc("OpenTraceW")
	procProcessTrace           = modadvapi32.NewProc("ProcessTrace")
	procCloseTrace             = modadvapi32.NewProc("CloseTrace")
	modtdh                     = windows.NewLazySystemDLL("tdh.dll")
	procTdhGetEventInformation = modtdh.NewProc("TdhGetEventInformation")
	procTdhGetPropertySize     = modtdh.NewProc("TdhGetPropertySize")
	procTdhGetProperty         = modtdh.NewProc("TdhGetProperty")
)

// Well-known USB stack providers.
var (
	ETWProviderUSBHUB3 = windows.GUID{Data1: 0xAC52AD17, Data2: 0xCC01, Data3: 0x4F85, Data4: [8]byte{0x8D, 0xF5, 0x4D, 0xCE, 0x43, 0x33, 0xC9, 0x9B}}
	ETWProviderUSBXHCI = windows.GUID{Data1: 0x30E1D284, Data2: 0x5D88, Data3: 0x459C, Data4: [8]byte{0x83, 0xFD, 0x63, 0x45, 0xB3, 0x9B, 0x19, 0xEC}}
	ETWProviderUCX     = windows.GUID{Data1: 0x36DA592D, Data2: 0xE43A, Data3: 0x4E28, Data4: [8]byte{0xAF, 0x6F, 0x4B, 0xC5, 0x7C, 0x5A, 0x11, 0xE8}}
	ETWProviderUSBPORT = windows.GUID{Data1: 0xC88A4EF5, Data2: 0xD048, Data3: 0x4013, Data4: [8]byte{0x94, 0x08, 0xE0, 0x4B, 0x7D, 0xB2, 0x81, 0x4A}}
	ETWProviderUSBHUB  = windows.GUID{Data1: 0x7426A56B, Data2: 0xE2D5, Data3: 0x4B30, Data4: [8]byte{0xBD, 0xEF, 0xB3, 0x18, 0x15, 0xC1, 0xA7, 0x4A}}
	ETWProviderUSBSTOR = windows.GUID{Data1: 0x72FB9358, Data2: 0xA9B3, Data3: 0x41E0, Data4: [8]byte{0xAE, 0x41, 0xE8, 0xDE, 0xCA, 0x41, 0xE3, 0xA8}}
)

// USB provider keywords shared by USBHUB3 / USBXHCI / UCX.
const (
	ETWKeywordDefault         = 0x1
	ETWKeywordUSBError        = 0x2
	ETWKeywordIRP             = 0x4
	ETWKeywordPerformance     = 0x20
	ETWKeywordHeadersBusTrace = 0x40
	ETWKeywordRundown         = 0x8000
	ETWKeywordDevice          = 0x10000
)

// ETWProvider is one provider to enable on the session.
type ETWProvider struct {
	Name     string
	GUID     windows.GUID
	Keywords uint64
	Level    uint8 // 5 = verbose
}

// ETWProperty is one decoded event payload field, in manifest order.
type ETWProperty struct {
	Name  string
	Value any
}

// ETWEvent is a decoded event record.
type ETWEvent struct {
	Provider   string
	ProviderID windows.GUID
	ID         uint16
	Version    uint8
	Opcode     uint8
	Task       uint16
	Keyword    uint64
	ProcessID  uint32
	At         time.Time
	TaskName   string
	OpcodeName string
	Properties []ETWProperty
	DecodeErr  string
}

// Property returns a decoded property by name. Struct members are
// addressed as "struct.member".
func (e *ETWEvent) Property(name string) (any, bool) {
	return lookupProperty(e.Properties, name)
}

func lookupProperty(props []ETWProperty, name string) (any, bool) {
	for _, p := range props {
		if p.Name == name {
			return p.Value, true
		}
		if members, ok := p.Value.([]ETWProperty); ok && strings.HasPrefix(name, p.Name+".") {
			return lookupProperty(members, name[len(p.Name)+1:])
		}
	}
	return nil, false
}

// ETWStats summarises a finished session.
type ETWStats struct {
	Events              uint64
	EventsLost          uint32
	RealTimeBuffersLost uint32
	Duration            time.Duration
}

// --- session structures -------------------------------------------------

type wnodeHeader struct {
	BufferSize        uint32
	ProviderID        uint32
	HistoricalContext uint64
	TimeStamp         int64
	GUID              windows.GUID
	ClientContext     uint32
	Flags             uint32
}

type eventTraceProperties struct {
	Wnode               wnodeHeader
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      uintptr
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

const maxSessionName = 1024

type sessionProperties struct {
	eventTraceProperties
	LoggerName  [maxSessionName]uint16
	LogFileName [maxSessionName]uint16
}

const (
	wnodeFlagTracedGUID       = 0x00020000
	eventTraceRealTimeMode    = 0x00000100
	eventTraceControlStop     = 1
	eventControlCodeEnable    = 1
	processTraceModeRealTime  = 0x00000100
	processTraceModeEventRec  = 0x10000000
	invalidProcessTraceHandle = ^uint64(0)
	errorInsufficientBuffer   = 122
	errorAlreadyExists        = 183
	errorAccessDenied         = 5
)

func newSessionProperties(name string) *sessionProperties {
	p := &sessionProperties{}
	p.Wnode.BufferSize = uint32(unsafe.Sizeof(*p))
	p.Wnode.Flags = wnodeFlagTracedGUID
	p.Wnode.ClientContext = 1 // QPC timestamps
	p.BufferSize = 64         // KB per buffer
	p.MinimumBuffers = 16
	p.MaximumBuffers = 64
	p.LogFileMode = eventTraceRealTimeMode
	p.LoggerNameOffset = uint32(unsafe.Offsetof(p.LoggerName))
	p.LogFileNameOffset = uint32(unsafe.Offsetof(p.LogFileName))
	copy(p.LoggerName[:maxSessionName-1], windows.StringToUTF16(name))
	return p
}

// --- consumer structures ------------------------------------------------

type eventTraceHeader struct {
	Size           uint16
	FieldTypeFlags uint16
	Class          [4]byte
	ThreadID       uint32
	ProcessID      uint32
	TimeStamp      int64
	GUID           windows.GUID
	ProcessorTime  uint64
}

type eventTrace struct {
	Header           eventTraceHeader
	InstanceID       uint32
	ParentInstanceID uint32
	ParentGUID       windows.GUID
	MofData          uintptr
	MofLength        uint32
	BufferContext    uint32
}

type systemTime struct {
	Year, Month, DayOfWeek, Day, Hour, Minute, Second, Milliseconds uint16
}

type timeZoneInformation struct {
	Bias         int32
	StandardName [32]uint16
	StandardDate systemTime
	StandardBias int32
	DaylightName [32]uint16
	DaylightDate systemTime
	DaylightBias int32
}

type traceLogfileHeader struct {
	BufferSize         uint32
	Version            uint32
	ProviderVersion    uint32
	NumberOfProcessors uint32
	EndTime            int64
	TimerResolution    uint32
	MaximumFileSize    uint32
	LogFileMode        uint32
	BuffersWritten     uint32
	LogInstanceGUID    windows.GUID
	LoggerName         uintptr
	LogFileName        uintptr
	TimeZone           timeZoneInformation
	BootTime           int64
	PerfFreq           int64
	StartTime          int64
	ReservedFlags      uint32
	BuffersLost        uint32
}

type eventTraceLogfile struct {
	LogFileName         *uint16
	LoggerName          *uint16
	CurrentTime         int64
	BuffersRead         uint32
	ProcessTraceMode    uint32
	CurrentEvent        eventTrace
	LogfileHeader       traceLogfileHeader
	BufferCallback      uintptr
	BufferSize          uint32
	Filled              uint32
	EventsLost          uint32
	EventRecordCallback uintptr
	IsKernelTrace       uint32
	Context             uintptr
}

type eventDescriptor struct {
	ID      uint16
	Version uint8
	Channel uint8
	Level   uint8
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

type eventHeader struct {
	Size            uint16
	HeaderType      uint16
	Flags           uint16
	EventProperty   uint16
	ThreadID        uint32
	ProcessID       uint32
	TimeStamp       int64
	ProviderID      windows.GUID
	EventDescriptor eventDescriptor
	ProcessorTime   uint64
	ActivityID      windows.GUID
}

type eventRecord struct {
	EventHeader       eventHeader
	BufferContext     uint32
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      uintptr
	UserData          uintptr
	UserContext       uintptr
}

type propertyDataDescriptor struct {
	PropertyName uint64
	ArrayIndex   uint32
	Reserved     uint32
}

// --- session lifecycle --------------------------------------------------

// etwConsumer is the single active consumer; ETW callbacks are process
// global so the dispatch goes through one package-level pointer.
var (
	consumerMu       sync.Mutex
	activeConsumer   *etwConsumer
	recordCallback   uintptr
	recordCallbackMu sync.Once
)

type etwConsumer struct {
	providers map[windows.GUID]string
	handle    func(*ETWEvent)
	events    uint64
}

func eventRecordCallback(rec *eventRecord) uintptr {
	consumerMu.Lock()
	c := activeConsumer
	consumerMu.Unlock()
	if c == nil || rec == nil {
		return 0
	}
	defer func() {
		if r := recover(); r != nil {
			// Never let a decode bug unwind through the ETW thread.
			_ = r
		}
	}()
	c.events++
	ev := &ETWEvent{
		ProviderID: rec.EventHeader.ProviderID,
		ID:         rec.EventHeader.EventDescriptor.ID,
		Version:    rec.EventHeader.EventDescriptor.Version,
		Opcode:     rec.EventHeader.EventDescriptor.Opcode,
		Task:       rec.EventHeader.EventDescriptor.Task,
		Keyword:    rec.EventHeader.EventDescriptor.Keyword,
		ProcessID:  rec.EventHeader.ProcessID,
		At:         time.Now(),
	}
	ev.Provider = c.providers[ev.ProviderID]
	if ev.Provider == "" {
		ev.Provider = ev.ProviderID.String()
	}
	if err := decodeProperties(rec, ev); err != nil {
		ev.DecodeErr = err.Error()
	}
	c.handle(ev)
	return 0
}

// RunETWSession starts a real-time session, enables the providers, and
// delivers decoded events to handle on the ETW thread until ctx is done.
// It blocks for the life of the session.
func RunETWSession(ctx context.Context, sessionName string, providers []ETWProvider, handle func(*ETWEvent)) (ETWStats, error) {
	var stats ETWStats
	if len(providers) == 0 {
		return stats, fmt.Errorf("etw: no providers")
	}
	name16, err := windows.UTF16PtrFromString(sessionName)
	if err != nil {
		return stats, err
	}

	// A crashed previous run leaves the session alive; stop it first.
	stopProps := newSessionProperties(sessionName)
	procControlTraceW.Call(0, uintptr(unsafe.Pointer(name16)), uintptr(unsafe.Pointer(stopProps)), eventTraceControlStop)

	props := newSessionProperties(sessionName)
	var session uint64
	ret, _, _ := procStartTraceW.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(name16)), uintptr(unsafe.Pointer(props)))
	switch ret {
	case 0:
	case errorAccessDenied:
		return stats, fmt.Errorf("etw: StartTrace access denied: run from an elevated (administrator) terminal")
	default:
		return stats, fmt.Errorf("etw: StartTrace failed: %w", windows.Errno(ret))
	}
	defer procControlTraceW.Call(uintptr(session), uintptr(unsafe.Pointer(name16)), uintptr(unsafe.Pointer(props)), eventTraceControlStop)

	names := map[windows.GUID]string{}
	for _, p := range providers {
		guid := p.GUID
		level := p.Level
		if level == 0 {
			level = 5
		}
		ret, _, _ := procEnableTraceEx2.Call(uintptr(session), uintptr(unsafe.Pointer(&guid)), eventControlCodeEnable,
			uintptr(level), uintptr(p.Keywords), 0, 0, 0)
		if ret != 0 {
			return stats, fmt.Errorf("etw: enable %s: %w", p.Name, windows.Errno(ret))
		}
		names[guid] = p.Name
	}

	recordCallbackMu.Do(func() {
		recordCallback = windows.NewCallback(eventRecordCallback)
	})
	consumer := &etwConsumer{providers: names, handle: handle}
	consumerMu.Lock()
	if activeConsumer != nil {
		consumerMu.Unlock()
		return stats, fmt.Errorf("etw: a session is already being consumed in this process")
	}
	activeConsumer = consumer
	consumerMu.Unlock()
	defer func() {
		consumerMu.Lock()
		activeConsumer = nil
		consumerMu.Unlock()
	}()

	logfile := &eventTraceLogfile{
		LoggerName:          name16,
		ProcessTraceMode:    processTraceModeRealTime | processTraceModeEventRec,
		EventRecordCallback: recordCallback,
	}
	h, _, _ := procOpenTraceW.Call(uintptr(unsafe.Pointer(logfile)))
	trace := uint64(h)
	if trace == invalidProcessTraceHandle {
		return stats, fmt.Errorf("etw: OpenTrace failed: %w", windows.GetLastError())
	}

	started := time.Now()
	done := make(chan uint64, 1)
	go func() {
		runtime.LockOSThread()
		handles := []uint64{trace}
		ret, _, _ := procProcessTrace.Call(uintptr(unsafe.Pointer(&handles[0])), 1, 0, 0)
		done <- uint64(ret)
	}()

	select {
	case <-ctx.Done():
	case ret := <-done:
		procCloseTrace.Call(uintptr(trace))
		return stats, fmt.Errorf("etw: ProcessTrace ended early: %w", windows.Errno(ret))
	}

	// Stopping the session makes ProcessTrace return once buffers drain.
	procControlTraceW.Call(uintptr(session), uintptr(unsafe.Pointer(name16)), uintptr(unsafe.Pointer(props)), eventTraceControlStop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		procCloseTrace.Call(uintptr(trace))
		<-done
	}
	procCloseTrace.Call(uintptr(trace))

	stats.Events = consumer.events
	stats.EventsLost = props.EventsLost
	stats.RealTimeBuffersLost = props.RealTimeBuffersLost
	stats.Duration = time.Since(started)
	return stats, nil
}

// --- TDH property decoding ----------------------------------------------

// TRACE_EVENT_INFO offsets (64-bit), per SDK 10.0.26100 tdh.h. Note the
// BinaryXMLSize field at 88 that older documentation omits.
const (
	teiTaskNameOffset      = 68
	teiOpcodeNameOffset    = 72
	teiPropertyCount       = 100
	teiTopLevelPropCount   = 104
	teiPropertyInfoArray   = 112
	eventPropertyInfoSize  = 24
	propertyStruct         = 0x1
	propertyParamCount     = 0x4
	propertyParamFixedCnt  = 0x20
	tdhInTypeUnicodeString = 1
	tdhInTypeAnsiString    = 2
	tdhInTypeInt8          = 3
	tdhInTypeUInt8         = 4
	tdhInTypeInt16         = 5
	tdhInTypeUInt16        = 6
	tdhInTypeInt32         = 7
	tdhInTypeUInt32        = 8
	tdhInTypeInt64         = 9
	tdhInTypeUInt64        = 10
	tdhInTypeFloat         = 11
	tdhInTypeDouble        = 12
	tdhInTypeBoolean       = 13
	tdhInTypeBinary        = 14
	tdhInTypeGUID          = 15
	tdhInTypePointer       = 16
	tdhInTypeHexInt32      = 20
	tdhInTypeHexInt64      = 21
)

func decodeProperties(rec *eventRecord, ev *ETWEvent) error {
	var size uint32
	ret, _, _ := procTdhGetEventInformation.Call(uintptr(unsafe.Pointer(rec)), 0, 0, 0, uintptr(unsafe.Pointer(&size)))
	if ret != errorInsufficientBuffer {
		return fmt.Errorf("TdhGetEventInformation: %w", windows.Errno(ret))
	}
	info := make([]byte, size)
	ret, _, _ = procTdhGetEventInformation.Call(uintptr(unsafe.Pointer(rec)), 0, 0, uintptr(unsafe.Pointer(&info[0])), uintptr(unsafe.Pointer(&size)))
	if ret != 0 {
		return fmt.Errorf("TdhGetEventInformation: %w", windows.Errno(ret))
	}
	ev.TaskName = wideAt(info, binary.LittleEndian.Uint32(info[teiTaskNameOffset:]))
	ev.OpcodeName = wideAt(info, binary.LittleEndian.Uint32(info[teiOpcodeNameOffset:]))

	top := int(binary.LittleEndian.Uint32(info[teiTopLevelPropCount:]))
	for i := 0; i < top; i++ {
		ev.Properties = append(ev.Properties, decodeProperty(rec, info, i, nil))
	}
	runtime.KeepAlive(info)
	return nil
}

// propertyInfo is one EVENT_PROPERTY_INFO entry (24 bytes):
//
//	 0 ULONG Flags, 4 ULONG NameOffset,
//	 8 USHORT InType | StructStartIndex, 10 USHORT OutType | NumOfStructMembers,
//	12 ULONG MapNameOffset, 16 USHORT count, 18 USHORT length, 20 ULONG Reserved
type propertyInfo struct {
	flags   uint32
	nameOff uint32
	inType  uint16 // or StructStartIndex when flags&propertyStruct
	outType uint16 // or NumOfStructMembers when flags&propertyStruct
	count   uint16
}

func readPropertyInfo(info []byte, index int) (propertyInfo, bool) {
	off := teiPropertyInfoArray + index*eventPropertyInfoSize
	if off+eventPropertyInfoSize > len(info) {
		return propertyInfo{}, false
	}
	pi := info[off:]
	return propertyInfo{
		flags:   binary.LittleEndian.Uint32(pi[0:]),
		nameOff: binary.LittleEndian.Uint32(pi[4:]),
		inType:  binary.LittleEndian.Uint16(pi[8:]),
		outType: binary.LittleEndian.Uint16(pi[10:]),
		count:   binary.LittleEndian.Uint16(pi[16:]),
	}, true
}

// decodeProperty renders property index. parent is the enclosing struct's
// descriptor when decoding a struct member; struct values are rendered as
// a nested []ETWProperty.
func decodeProperty(rec *eventRecord, info []byte, index int, parent *propertyDataDescriptor) ETWProperty {
	pi, ok := readPropertyInfo(info, index)
	if !ok {
		return ETWProperty{Name: fmt.Sprintf("<property %d out of range>", index)}
	}
	prop := ETWProperty{Name: wideAt(info, pi.nameOff)}
	if int(pi.nameOff) >= len(info) {
		prop.Value = "<bad name offset>"
		return prop
	}
	self := propertyDataDescriptor{
		PropertyName: uint64(uintptr(unsafe.Pointer(&info[pi.nameOff]))),
		ArrayIndex:   ^uint32(0),
	}
	switch {
	case pi.flags&propertyStruct != 0:
		if parent != nil {
			prop.Value = "<nested struct>"
			return prop
		}
		// Members are addressed as (struct, member); struct ArrayIndex 0.
		self.ArrayIndex = 0
		members := make([]ETWProperty, 0, pi.outType)
		for j := 0; j < int(pi.outType); j++ {
			members = append(members, decodeProperty(rec, info, int(pi.inType)+j, &self))
		}
		prop.Value = members
	case pi.flags&(propertyParamCount|propertyParamFixedCnt) != 0 || pi.count > 1:
		prop.Value = fmt.Sprintf("<array x%d>", pi.count)
	default:
		descriptors := []propertyDataDescriptor{self}
		if parent != nil {
			descriptors = []propertyDataDescriptor{*parent, self}
		}
		raw, err := getProperty(rec, descriptors)
		if err != nil {
			prop.Value = "<" + err.Error() + ">"
		} else {
			prop.Value = renderProperty(pi.inType, raw)
		}
	}
	return prop
}

func getProperty(rec *eventRecord, descriptors []propertyDataDescriptor) ([]byte, error) {
	n := uintptr(len(descriptors))
	pdd := uintptr(unsafe.Pointer(&descriptors[0]))
	var size uint32
	ret, _, _ := procTdhGetPropertySize.Call(uintptr(unsafe.Pointer(rec)), 0, 0, n, pdd, uintptr(unsafe.Pointer(&size)))
	if ret != 0 {
		return nil, fmt.Errorf("TdhGetPropertySize: %w", windows.Errno(ret))
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	ret, _, _ = procTdhGetProperty.Call(uintptr(unsafe.Pointer(rec)), 0, 0, n, pdd, uintptr(size), uintptr(unsafe.Pointer(&buf[0])))
	if ret != 0 {
		return nil, fmt.Errorf("TdhGetProperty: %w", windows.Errno(ret))
	}
	runtime.KeepAlive(descriptors)
	return buf, nil
}

func renderProperty(inType uint16, raw []byte) any {
	le := binary.LittleEndian
	switch inType {
	case tdhInTypeUnicodeString:
		return decodeWide(raw)
	case tdhInTypeAnsiString:
		for i, b := range raw {
			if b == 0 {
				return string(raw[:i])
			}
		}
		return string(raw)
	case tdhInTypeInt8:
		if len(raw) >= 1 {
			return int64(int8(raw[0]))
		}
	case tdhInTypeUInt8:
		if len(raw) >= 1 {
			return uint64(raw[0])
		}
	case tdhInTypeInt16:
		if len(raw) >= 2 {
			return int64(int16(le.Uint16(raw)))
		}
	case tdhInTypeUInt16:
		if len(raw) >= 2 {
			return uint64(le.Uint16(raw))
		}
	case tdhInTypeInt32:
		if len(raw) >= 4 {
			return int64(int32(le.Uint32(raw)))
		}
	case tdhInTypeUInt32, tdhInTypeHexInt32:
		if len(raw) >= 4 {
			return uint64(le.Uint32(raw))
		}
	case tdhInTypeInt64:
		if len(raw) >= 8 {
			return int64(le.Uint64(raw))
		}
	case tdhInTypeUInt64, tdhInTypeHexInt64, tdhInTypePointer:
		if len(raw) >= 8 {
			return le.Uint64(raw)
		}
		if len(raw) >= 4 {
			return uint64(le.Uint32(raw))
		}
	case tdhInTypeBoolean:
		if len(raw) >= 4 {
			return le.Uint32(raw) != 0
		}
	case tdhInTypeGUID:
		if len(raw) >= 16 {
			return (*windows.GUID)(unsafe.Pointer(&raw[0])).String()
		}
	}
	if len(raw) > 32 {
		return hex.EncodeToString(raw[:32]) + fmt.Sprintf("...(%d bytes)", len(raw))
	}
	return hex.EncodeToString(raw)
}

// wideAt reads a NUL-terminated UTF-16 string at a byte offset into buf.
func wideAt(buf []byte, off uint32) string {
	if off == 0 || int(off) >= len(buf) {
		return ""
	}
	return decodeWide(buf[off:])
}
