package storage

import (
	"bytes"
	"crypto/rand"
	"errors"
	"image"
	"image/color"
	"image/png"
)

func errorsNew(msg string) error { return errors.New(msg) }

func errorsAs(err error, target any) bool { return errors.As(err, target) }

func cryptoRandRead(b []byte) (int, error) { return rand.Read(b) }

// buildProbePNG 生成 64x64 双色探测图（连通性检测用）。
func buildProbePNG() []byte {
	picture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			shade := color.RGBA{15, 118, 110, 255}
			if x >= 14 && x < 50 && y >= 14 && y < 50 {
				shade = color.RGBA{240, 253, 250, 255}
			}
			picture.Set(x, y, shade)
		}
	}
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, picture)
	return buffer.Bytes()
}
