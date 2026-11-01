package service

import (
	"bytes"
	"encoding/binary"
)

// imageOrientation reads only the bounded IFD0 Orientation entry. It does not
// follow nested metadata pointers or embedded EXIF thumbnails.
func imageOrientation(data []byte) int {
	if bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		for p := 2; p < len(data); {
			if data[p] != 0xff {
				break
			}
			for p < len(data) && data[p] == 0xff {
				p++
			}
			if p >= len(data) {
				break
			}
			marker := data[p]
			p++
			if marker == 0xda || marker == 0xd9 {
				break
			}
			if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
				continue
			}
			if len(data)-p < 2 {
				break
			}
			n := int(binary.BigEndian.Uint16(data[p : p+2]))
			if n < 2 || n > len(data)-p {
				break
			}
			payload := data[p+2 : p+n]
			if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				return tiffOrientation(payload[6:])
			}
			p += n
		}
	} else if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		for p := 8; len(data)-p >= 12; {
			n := uint64(binary.BigEndian.Uint32(data[p : p+4]))
			if n > uint64(len(data)-p-12) {
				break
			}
			if string(data[p+4:p+8]) == "eXIf" {
				return tiffOrientation(data[p+8 : p+8+int(n)])
			}
			p += 12 + int(n)
		}
	} else if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		end := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
		if end > uint64(len(data)) {
			return 1
		}
		data = data[:int(end)]
		for p := 12; len(data)-p >= 8; {
			n := uint64(binary.LittleEndian.Uint32(data[p+4 : p+8]))
			if n+n%2 > uint64(len(data)-p-8) {
				break
			}
			if string(data[p:p+4]) == "EXIF" {
				payload := data[p+8 : p+8+int(n)]
				payload = bytes.TrimPrefix(payload, []byte("Exif\x00\x00"))
				return tiffOrientation(payload)
			}
			p += 8 + int(n+n%2)
		}
	}
	return 1
}

func tiffOrientation(data []byte) int {
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1
	}
	p := uint64(order.Uint32(data[4:8]))
	if p < 8 || p+2 > uint64(len(data)) {
		return 1
	}
	n := uint64(order.Uint16(data[p : p+2]))
	p += 2
	if uint64(len(data))-p < 4 || n > (uint64(len(data))-p-4)/12 {
		return 1
	}
	for i := uint64(0); i < n; i++ {
		entry := data[p+i*12 : p+(i+1)*12]
		if order.Uint16(entry[:2]) != 0x0112 {
			continue
		}
		if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
			return 1
		}
		value := int(order.Uint16(entry[8:10]))
		if value >= 1 && value <= 8 {
			return value
		}
		return 1
	}
	return 1
}

func orientedSize(width, height, orientation int) (int, int) {
	if orientation >= 5 && orientation <= 8 {
		return height, width
	}
	return width, height
}

func orientedSource(x, y, width, height, orientation int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return y, height - 1 - x
	case 7:
		return width - 1 - y, height - 1 - x
	case 8:
		return width - 1 - y, x
	default:
		return x, y
	}
}
