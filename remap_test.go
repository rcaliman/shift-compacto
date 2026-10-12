package main

import (
	"reflect"
	"testing"
)

func TestRemap(t *testing.T) {
	tests := []struct {
		name        string
		input, want []keyEvent
	}{
		{"barra ao soltar", []keyEvent{{89, 1}, {89, 0}}, []keyEvent{{54, 1}, {54, 0}, {89, 1}, {89, 0}}},
		{"Shift com letra", []keyEvent{{89, 1}, {30, 1}, {30, 0}, {89, 0}}, []keyEvent{{54, 1}, {30, 1}, {30, 0}, {54, 0}}},
		{"interrogacao", []keyEvent{{42, 1}, {89, 1}, {89, 2}, {89, 0}, {42, 0}}, []keyEvent{{42, 1}, {89, 1}, {89, 2}, {89, 0}, {42, 0}}},
		{"repeticao sozinha", []keyEvent{{89, 1}, {89, 2}, {89, 1}, {89, 0}}, []keyEvent{{54, 1}, {54, 0}, {89, 1}, {89, 0}}},
		{"soltar esquerdo primeiro", []keyEvent{{42, 1}, {89, 1}, {42, 0}, {89, 0}}, []keyEvent{{42, 1}, {89, 1}, {42, 0}, {89, 0}}},
		{"toque depois de Shift", []keyEvent{{89, 1}, {30, 1}, {89, 0}, {30, 0}, {89, 1}, {89, 0}}, []keyEvent{{54, 1}, {30, 1}, {54, 0}, {30, 0}, {54, 1}, {54, 0}, {89, 1}, {89, 0}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := remap{key: 89, left: 42}
			var got []keyEvent
			for _, e := range tc.input {
				got = append(got, r.event(e.code, e.value)...)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v; want %v", got, tc.want)
			}
		})
	}
}
