package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type dbfLayout struct {
	headerSize, recordSize int
	count                  uint64
	fields                 map[string]cashField
}

func readDBFLayout(data []byte) (dbfLayout, error) {
	invalid := func() (dbfLayout, error) {
		return dbfLayout{}, fmt.Errorf("struktur file tidak valid atau tidak didukung")
	}
	if len(data) < 33 || (data[0] != 0x03 && data[0] != 0x83) {
		return invalid()
	}
	h := int(binary.LittleEndian.Uint16(data[8:10]))
	size := int(binary.LittleEndian.Uint16(data[10:12]))
	count := uint64(binary.LittleEndian.Uint32(data[4:8]))
	if h < 33 || h > len(data) || (h-33)%32 != 0 || size < 2 || data[h-1] != 0x0d {
		return invalid()
	}
	if count*uint64(size) > uint64(len(data)-h) {
		return invalid()
	}
	fields := map[string]cashField{}
	offset := 1
	for i := 32; i < h-1; i += 32 {
		name := string(bytes.TrimRight(data[i:i+11], "\x00"))
		length := int(data[i+16])
		if name == "" || length == 0 {
			return invalid()
		}
		if _, exists := fields[name]; exists {
			return invalid()
		}
		fields[name] = cashField{name, data[i+11], length, int(data[i+17]), offset}
		offset += length
	}
	if offset != size {
		return invalid()
	}
	return dbfLayout{h, size, count, fields}, nil
}
