package main

import (
	"flag"
	"fmt"
	"os"
)

type options struct {
	key, left int
	list      bool
	check     bool
	watch     bool
	device    string
}

type keyEvent struct {
	code  uint16
	value int32
}
type remap struct {
	key, left      uint16
	leftDown, used bool
	mode           int // 0=solta, 1=Shift, 2=tecla original
}

func (r *remap) event(code uint16, value int32) []keyEvent {
	if code == r.left {
		r.leftDown = value != 0
	}
	if code != r.key {
		if r.mode == 1 && value != 0 {
			r.used = true
		}
		return []keyEvent{{code, value}}
	}
	if value == 0 {
		mode := r.mode
		r.mode = 0
		if mode == 1 {
			out := []keyEvent{{54, 0}}
			if !r.used {
				out = append(out, keyEvent{code, 1}, keyEvent{code, 0})
			}
			return out
		}
		if mode == 2 {
			return []keyEvent{{code, 0}}
		}
		return nil
	}
	if r.mode == 0 {
		r.used = false
		if r.leftDown {
			r.mode = 2
			return []keyEvent{{code, 1}}
		}
		r.mode = 1
		return []keyEvent{{54, 1}}
	}
	if r.mode == 2 {
		return []keyEvent{{code, value}}
	}
	return nil
}

func main() {
	var o options
	flag.IntVar(&o.key, "key", 89, "Código da tecla /?")
	flag.IntVar(&o.left, "left-shift", 42, "Código do Shift esquerdo")
	flag.BoolVar(&o.list, "list", false, "Lista teclados físicos no Linux")
	flag.BoolVar(&o.check, "check", false, "Linux: testa criação dos teclados virtuais sem capturar teclas")
	flag.BoolVar(&o.watch, "watch", false, "Linux: acompanha conexão e reconexão de teclados (para serviço)")
	flag.StringVar(&o.device, "device", "", "Linux: restringir a /dev/input/eventX; padrão: todos")
	flag.Parse()
	if o.key < 1 || o.key > 767 || o.left < 1 || o.left > 767 || o.key == o.left || o.key == 54 || o.left == 54 {
		fmt.Fprintln(os.Stderr, "Códigos inválidos ou coincidentes com Shift direito (54).")
		os.Exit(2)
	}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "Erro:", err)
		os.Exit(1)
	}
}
