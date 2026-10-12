//go:build linux && (amd64 || arm64)

package main

import (
	"context"
	"encoding/binary"
	"os"
	"reflect"
	"syscall"
	"testing"
)

func TestLinuxEventStream(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	fd := reader.Fd()
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		t.Fatal(err)
	}
	output, err := os.CreateTemp(t.TempDir(), "events")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	k := &linuxKeyboard{input: reader, fd: fd, output: output, state: remap{key: 89, left: 42}, pressed: make(map[uint16]bool)}
	var stream []byte
	for _, e := range []struct {
		typ, code uint16
		value     int32
	}{{1, 89, 1}, {0, 0, 0}, {1, 89, 0}, {0, 0, 0}} {
		var b [24]byte
		binary.NativeEndian.PutUint16(b[16:18], e.typ)
		binary.NativeEndian.PutUint16(b[18:20], e.code)
		binary.NativeEndian.PutUint32(b[20:24], uint32(e.value))
		stream = append(stream, b[:]...)
	}
	if _, err := writer.Write(stream); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	if err := k.listen(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got []keyEvent
	syn := 0
	for i := 0; i < len(b); i += 24 {
		if binary.NativeEndian.Uint16(b[i+16:i+18]) == 0 {
			syn++
			continue
		}
		got = append(got, keyEvent{binary.NativeEndian.Uint16(b[i+18 : i+20]), int32(binary.NativeEndian.Uint32(b[i+20 : i+24]))})
	}
	want := []keyEvent{{54, 1}, {54, 0}, {89, 1}, {89, 0}}
	if !reflect.DeepEqual(got, want) || syn != 2 || len(k.pressed) != 0 {
		t.Fatalf("eventos=%v syn=%d pressed=%v", got, syn, k.pressed)
	}
}
