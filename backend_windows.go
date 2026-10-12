//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var setHook = user32.NewProc("SetWindowsHookExW")
var nextHook = user32.NewProc("CallNextHookEx")
var unhook = user32.NewProc("UnhookWindowsHookEx")
var sendInput = user32.NewProc("SendInput")
var getMessage = user32.NewProc("GetMessageW")
var postMessage = user32.NewProc("PostThreadMessageW")
var peekMessage = user32.NewProc("PeekMessageW")
var getKeyState = user32.NewProc("GetAsyncKeyState")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getThreadID = kernel32.NewProc("GetCurrentThreadId")
var getModule = kernel32.NewProc("GetModuleHandleW")

const injectedMarker = 0x53484347

type hookData struct {
	vk, scan, flags, time uint32
	extra                 uintptr
}
type winInput struct {
	typ         uint32
	pad         uint32
	vk, scan    uint16
	flags, time uint32
	pad2        uint32
	extra       uintptr
	tail        [8]byte // tamanho da união INPUT, incluindo MOUSEINPUT
}
type winMessage struct {
	hwnd           uintptr
	message        uint32
	wparam, lparam uintptr
	time           uint32
	x, y           int32
	private        uint32
}

func inject(e keyEvent) error {
	i := winInput{typ: 1, scan: e.code, flags: 8, extra: injectedMarker} // KEYEVENTF_SCANCODE
	if e.value == 0 {
		i.flags |= 2
	}
	n, _, err := sendInput.Call(1, uintptr(unsafe.Pointer(&i)), unsafe.Sizeof(i))
	runtime.KeepAlive(i)
	if n != 1 {
		return fmt.Errorf("SendInput falhou: %v; para atuar em programas elevados, execute como administrador", err)
	}
	return nil
}

func run(o options) error {
	if o.list || o.check || o.watch || o.device != "" {
		return fmt.Errorf("--list, --check, --watch e --device estão disponíveis apenas no Linux")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread, _, _ := getThreadID.Call()
	var msg winMessage
	peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0) // cria fila de mensagens
	state := remap{key: uint16(o.key), left: uint16(o.left)}
	if o.left == 42 {
		down, _, _ := getKeyState.Call(0xA0)
		state.leftDown = down&0x8000 != 0
	}
	var hook uintptr
	var first error
	callback := syscall.NewCallback(func(code int32, message uintptr, data *hookData) uintptr {
		forward := func() uintptr {
			result, _, _ := nextHook.Call(hook, uintptr(code), message, uintptr(unsafe.Pointer(data)))
			return result
		}
		if code < 0 {
			return forward()
		}
		if data.flags&0x10 != 0 && data.extra == injectedMarker {
			return forward()
		}
		if message != 0x100 && message != 0x101 && message != 0x104 && message != 0x105 {
			return forward()
		}
		value := int32(1)
		if data.flags&0x80 != 0 {
			value = 0
		}
		scan := uint16(data.scan)
		if data.flags&1 != 0 {
			scan |= 0xE000
		} // separa teclas estendidas com mesmo scan code
		previous := state.mode
		out := state.event(scan, value)
		if scan != state.key || previous == 2 || state.mode == 2 {
			return forward()
		}
		for _, e := range out {
			if err := inject(e); err != nil {
				if first == nil {
					first = err
				}
				postMessage.Call(thread, 0x12, 0, 0)
				break
			}
		}
		return 1
	})
	module, _, _ := getModule.Call(0)
	var err error
	hook, _, err = setHook.Call(13, callback, module, 0) // WH_KEYBOARD_LL
	if hook == 0 {
		return fmt.Errorf("SetWindowsHookExW: %v", err)
	}
	defer func() {
		unhook.Call(hook)
		if state.mode == 1 {
			_ = inject(keyEvent{54, 0})
		}
	}()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-interrupt:
			postMessage.Call(thread, 0x12, 0, 0)
		case <-done:
		}
	}()
	fmt.Println("Ativo em todos os teclados. Ctrl+C neste terminal para encerrar.")
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessageW: %v", err)
		}
		if result == 0 {
			break
		}
	}
	runtime.KeepAlive(callback)
	return first
}
