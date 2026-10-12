//go:build linux && (amd64 || arm64)

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func ioc(dir, kind, nr, size uintptr) uintptr { return dir<<30 | size<<16 | kind<<8 | nr }
func ioctl(fd uintptr, req, arg uintptr) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	if e != 0 {
		return e
	}
	return nil
}
func query(fd uintptr, nr uintptr, size int) ([]byte, error) {
	b := make([]byte, size)
	err := ioctl(fd, ioc(2, 'E', nr, uintptr(size)), uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
	return b, err
}
func has(bits []byte, code int) bool {
	return code >= 0 && code/8 < len(bits) && bits[code/8]&(1<<uint(code%8)) != 0
}

type linuxKeyboard struct {
	input, output *os.File
	fd            uintptr
	name          string
	state         remap
	pressed       map[uint16]bool
	grabbed       bool
}

func openKeyboard(path string, o options) (*linuxKeyboard, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Join("/sys/class/input", filepath.Base(path), "device"))
	if err != nil {
		return nil, err
	}
	if strings.Contains(resolved, "/devices/virtual/input/") {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	fd := f.Fd()
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		return nil, err
	}
	keys, err := query(fd, 0x21, 96)
	if err != nil {
		return nil, err
	}
	if !has(keys, o.key) || !has(keys, o.left) || !has(keys, 30) {
		return nil, nil
	}
	name, err := query(fd, 0x06, 256)
	if err != nil {
		return nil, err
	}
	ok = true
	return &linuxKeyboard{input: f, fd: fd, name: strings.TrimRight(string(name), "\x00"), state: remap{key: uint16(o.key), left: uint16(o.left)}, pressed: make(map[uint16]bool)}, nil
}

func (k *linuxKeyboard) setup() error {
	active, err := query(k.fd, 0x18, 96)
	if err != nil {
		return err
	}
	for _, b := range active {
		if b != 0 {
			return fmt.Errorf("solte todas as teclas de %s antes de iniciar", k.name)
		}
	}
	k.output, err = os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("/dev/uinput: %w (execute com sudo)", err)
	}
	fd := k.output.Fd()
	types, err := query(k.fd, 0x20, 4)
	if err != nil {
		return err
	}
	// Não ativa EV_REP: os eventos de repetição já vêm do dispositivo físico.
	setters := map[int]uintptr{1: 101, 2: 102, 3: 103, 4: 104, 5: 109, 17: 105, 18: 106}
	for typ := 1; typ < 32; typ++ {
		setter, supported := setters[typ]
		if !has(types, typ) || !supported {
			continue
		}
		if err := ioctl(fd, ioc(1, 'U', 100, 4), uintptr(typ)); err != nil {
			return err
		}
		bits, err := query(k.fd, uintptr(0x20+typ), 96)
		if err != nil {
			return err
		}
		if typ == 1 {
			bits[54/8] |= 1 << uint(54%8)
		}
		for code := 0; code < len(bits)*8; code++ {
			if !has(bits, code) {
				continue
			}
			if err := ioctl(fd, ioc(1, 'U', setter, 4), uintptr(code)); err != nil {
				return err
			}
			if typ == 3 {
				info, err := query(k.fd, uintptr(0x40+code), 24)
				if err != nil {
					return err
				}
				var setup [28]byte
				binary.NativeEndian.PutUint16(setup[:2], uint16(code))
				copy(setup[4:], info)
				err = ioctl(fd, ioc(1, 'U', 4, 28), uintptr(unsafe.Pointer(&setup[0])))
				runtime.KeepAlive(setup)
				if err != nil {
					return err
				}
			}
		}
	}
	var setup [92]byte
	binary.NativeEndian.PutUint16(setup[:2], 6) // BUS_VIRTUAL
	copy(setup[8:88], "Shift compacto Go: "+k.name)
	setup[87] = 0
	err = ioctl(fd, ioc(1, 'U', 3, 92), uintptr(unsafe.Pointer(&setup[0])))
	runtime.KeepAlive(setup)
	if err != nil {
		return err
	}
	if err := ioctl(fd, ioc(0, 'U', 1, 0), 0); err != nil {
		return err
	}
	return nil
}

func (k *linuxKeyboard) write(typ, code uint16, value int32) error {
	var b [24]byte // input_event: timeval (16), type (2), code (2), value (4)
	binary.NativeEndian.PutUint16(b[16:18], typ)
	binary.NativeEndian.PutUint16(b[18:20], code)
	binary.NativeEndian.PutUint32(b[20:24], uint32(value))
	n, err := k.output.Write(b[:])
	if err == nil && n != len(b) {
		return fmt.Errorf("escrita incompleta em uinput")
	}
	return err
}
func (k *linuxKeyboard) close() {
	if k.input == nil {
		return
	}
	if k.output != nil {
		for code := range k.pressed {
			_ = k.write(1, code, 0)
		}
		_ = k.write(0, 0, 0)
		_ = ioctl(k.output.Fd(), ioc(0, 'U', 2, 0), 0)
		k.output.Close()
	}
	if k.grabbed {
		_ = ioctl(k.fd, ioc(1, 'E', 0x90, 4), 0)
	}
	k.input.Close()
}
func (k *linuxKeyboard) listen(ctx context.Context) error {
	b := make([]byte, 24*64)
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := syscall.Read(int(k.fd), b)
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Millisecond):
			}
			continue
		}
		if errors.Is(err, syscall.ENODEV) {
			fmt.Printf("Desconectado: %s. Reinicie após reconectar.\n", k.name)
			return nil
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if n%24 != 0 {
			return fmt.Errorf("input_event incompleto")
		}
		for i := 0; i < n; i += 24 {
			typ := binary.NativeEndian.Uint16(b[i+16 : i+18])
			code := binary.NativeEndian.Uint16(b[i+18 : i+20])
			value := int32(binary.NativeEndian.Uint32(b[i+20 : i+24]))
			if typ == 0 && code == 3 {
				return fmt.Errorf("eventos perdidos em %s; reinicie", k.name)
			}
			if typ == 1 {
				for _, e := range k.state.event(code, value) {
					if err := k.write(1, e.code, e.value); err != nil {
						return err
					}
					if e.value == 0 {
						delete(k.pressed, e.code)
					} else {
						k.pressed[e.code] = true
					}
				}
			} else if typ != 20 && typ != 21 {
				if err := k.write(typ, code, value); err != nil {
					return err
				}
			}
		}
	}
}

func run(o options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		err := runSession(ctx, o)
		if ctx.Err() != nil {
			return nil
		}
		if !o.watch || o.list || o.check {
			return err
		}
		if err != nil {
			fmt.Println("Aguardando teclados:", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func keyboardPaths(o options) ([]string, error) {
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, err
	}
	if o.device != "" {
		paths = []string{o.device}
	}
	var found []string
	for _, path := range paths {
		k, err := openKeyboard(path, o)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENODEV) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if k != nil {
			found = append(found, path)
			k.close()
		}
	}
	return found, nil
}

func runSession(parent context.Context, o options) error {
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return err
	}
	if o.device != "" {
		paths = []string{o.device}
	}
	var keyboards []*linuxKeyboard
	defer func() {
		for _, k := range keyboards {
			k.close()
		}
	}()
	for _, path := range paths {
		k, err := openKeyboard(path, o)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENODEV) {
				continue
			}
			if o.device != "" || !errors.Is(err, os.ErrPermission) {
				return fmt.Errorf("%s: %w", path, err)
			}
			continue
		}
		if k != nil {
			keyboards = append(keyboards, k)
		}
	}
	if len(keyboards) == 0 {
		return fmt.Errorf("nenhum teclado físico compatível acessível; conecte o teclado e execute com sudo")
	}
	if o.list {
		for _, k := range keyboards {
			fmt.Printf("%s: %s\n", k.input.Name(), k.name)
		}
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	for _, k := range keyboards {
		if ctx.Err() != nil {
			return nil
		}
		if err := k.setup(); err != nil {
			return fmt.Errorf("preparar %s: %w", k.name, err)
		}
	}
	if o.check {
		for _, k := range keyboards {
			fmt.Printf("OK: teclado virtual criado para %s; nenhuma tecla capturada.\n", k.name)
		}
		return nil
	}
	// Dá tempo para o compositor registrar os novos teclados antes da captura.
	time.Sleep(300 * time.Millisecond)
	for _, k := range keyboards {
		if ctx.Err() != nil {
			return nil
		}
		if err := ioctl(k.fd, ioc(1, 'E', 0x90, 4), 1); err != nil {
			return fmt.Errorf("capturar %s (%s): %w", k.name, k.input.Name(), err)
		}
		k.grabbed = true
		fmt.Printf("Ativo em %s (%s)\n", k.name, k.input.Name())
	}
	if o.watch {
		fmt.Println("Acompanhando conexão e reconexão de teclados.")
	} else {
		fmt.Println("Ctrl+C para encerrar. Reinicie após conectar ou reconectar um teclado.")
	}
	results := make(chan error, len(keyboards))
	var wg sync.WaitGroup
	if o.watch {
		var initial []string
		for _, k := range keyboards {
			initial = append(initial, k.input.Name())
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					current, err := keyboardPaths(o)
					if err == nil && strings.Join(initial, "\n") != strings.Join(current, "\n") {
						fmt.Println("Teclados conectados mudaram; atualizando captura.")
						cancel()
						return
					}
				}
			}
		}()
	}
	for _, k := range keyboards {
		wg.Add(1)
		go func(k *linuxKeyboard) {
			defer wg.Done()
			err := k.listen(ctx)
			k.close()
			k.input = nil
			k.output = nil
			results <- err
		}(k)
	}
	var first error
	for range keyboards {
		if err := <-results; err != nil && first == nil {
			first = err
			cancel()
		}
	}
	cancel()
	wg.Wait()
	// As goroutines já fecharam os dispositivos.
	keyboards = nil
	return first
}
