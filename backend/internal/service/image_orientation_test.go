package service

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func orientationTIFF(value int, order binary.ByteOrder) []byte {
	b := make([]byte, 26)
	copy(b, "II")
	if order == binary.BigEndian {
		copy(b, "MM")
	}
	order.PutUint16(b[2:], 42)
	order.PutUint32(b[4:], 8)
	order.PutUint16(b[8:], 1)
	order.PutUint16(b[10:], 0x0112)
	order.PutUint16(b[12:], 3)
	order.PutUint32(b[14:], 1)
	order.PutUint16(b[18:], uint16(value))
	return b
}

func orientationJPEG(data []byte, value int, order binary.ByteOrder) []byte {
	payload := append([]byte("Exif\x00\x00"), orientationTIFF(value, order)...)
	out := append([]byte{}, data[:2]...)
	out = append(out, 0xff, 0xe1, byte((len(payload)+2)>>8), byte(len(payload)+2))
	out = append(out, payload...)
	return append(out, data[2:]...)
}

func TestThumbnailCameraDirections(t *testing.T) {
	// Four independently labelled corners catch mirror, transpose and rotation.
	src := image.NewRGBA(image.Rect(0, 0, 80, 48))
	colors := []color.RGBA{{240, 20, 20, 255}, {20, 240, 20, 255}, {20, 20, 240, 255}, {230, 230, 20, 255}}
	for y := 0; y < 48; y++ {
		for x := 0; x < 80; x++ {
			src.Set(x, y, colors[(y/24)*2+x/40])
		}
	}
	var raw bytes.Buffer
	if err := jpeg.Encode(&raw, src, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	// Expected corner order: top-left, top-right, bottom-left, bottom-right.
	want := [8][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	for o := 1; o <= 8; o++ {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			t.Run(fmt.Sprintf("%d-%T", o, order), func(t *testing.T) {
				data := orientationJPEG(raw.Bytes(), o, order)
				saved := bytes.Clone(data)
				thumb, ok := buildThumb(data)
				if !ok {
					t.Fatal("thumbnail failed")
				}
				out, _, err := image.Decode(bytes.NewReader(thumb))
				if err != nil {
					t.Fatal(err)
				}
				w, h := 80, 48
				if o >= 5 {
					w, h = 48, 80
				}
				if out.Bounds().Dx() != w || out.Bounds().Dy() != h {
					t.Fatalf("wrong dimensions: %v", out.Bounds())
				}
				points := []image.Point{{w / 4, h / 4}, {3 * w / 4, h / 4}, {w / 4, 3 * h / 4}, {3 * w / 4, 3 * h / 4}}
				for i, p := range points {
					got := color.RGBAModel.Convert(out.At(p.X, p.Y)).(color.RGBA)
					c := colors[want[o-1][i]]
					if abs(int(got.R)-int(c.R)) > 20 || abs(int(got.G)-int(c.G)) > 20 || abs(int(got.B)-int(c.B)) > 20 {
						t.Fatalf("corner %d = %v, want %v", i, got, c)
					}
				}
				if !bytes.Equal(data, saved) || imageOrientation(thumb) != 1 {
					t.Fatal("original changed or thumbnail retains direction")
				}
			})
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestOrientationContainersAndMalformedMetadata(t *testing.T) {
	tiff := orientationTIFF(6, binary.LittleEndian)
	var raw bytes.Buffer
	png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 24, 16)))
	chunk := make([]byte, 12+len(tiff))
	binary.BigEndian.PutUint32(chunk, uint32(len(tiff)))
	copy(chunk[4:], "eXIf")
	copy(chunk[8:], tiff)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	pngData := append(bytes.Clone(raw.Bytes()[:33]), chunk...)
	pngData = append(pngData, raw.Bytes()[33:]...)
	thumb, ok := buildThumb(pngData)
	if !ok {
		t.Fatal("PNG thumbnail failed")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil || cfg.Width != 16 || cfg.Height != 24 {
		t.Fatal("PNG direction not applied", cfg, err)
	}
	for _, prefix := range []string{"", "Exif\x00\x00"} {
		payload := append([]byte(prefix), tiff...)
		payload = append(payload, 0) // exercise RIFF padding
		webp := append([]byte("RIFF\x00\x00\x00\x00WEBPEXIF\x00\x00\x00\x00"), payload...)
		binary.LittleEndian.PutUint32(webp[16:], uint32(len(payload)))
		if len(payload)%2 != 0 {
			webp = append(webp, 0)
		}
		binary.LittleEndian.PutUint32(webp[4:], uint32(len(webp)-8))
		if imageOrientation(webp) != 6 {
			t.Fatal("WebP direction not read")
		}
	}
	for n := 0; n < len(tiff); n++ {
		if tiffOrientation(tiff[:n]) != 1 {
			t.Fatal("truncated TIFF accepted")
		}
	}
	for _, o := range []int{0, 9, 65535} {
		if tiffOrientation(orientationTIFF(o, binary.BigEndian)) != 1 {
			t.Fatal("invalid orientation accepted")
		}
	}
	bad := bytes.Clone(tiff)
	binary.LittleEndian.PutUint32(bad[4:], 0xfffffff0)
	if tiffOrientation(bad) != 1 {
		t.Fatal("invalid offset accepted")
	}
	if imageOrientation(raw.Bytes()) != 1 {
		t.Fatal("plain image direction changed")
	}
	if _, ok := buildThumb(make([]byte, thumbDecodeMax+1)); ok {
		t.Fatal("oversized preview accepted")
	}
}

func FuzzImageOrientation(f *testing.F) {
	f.Add([]byte{})
	f.Add(orientationJPEG([]byte{0xff, 0xd8, 0xff, 0xd9}, 6, binary.LittleEndian))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("RIFF\xff\xff\xff\xffWEBP"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if o := imageOrientation(data); o < 1 || o > 8 {
			t.Fatalf("invalid orientation %d", o)
		}
	})
}
