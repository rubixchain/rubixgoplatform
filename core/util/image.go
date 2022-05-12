package util

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

func CreatePNGImage(pixels []byte, width int, height int, file string) error {
	if len(pixels) != width*height*3 {
		return fmt.Errorf("invalid pixel buffer")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	offset := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: pixels[offset],
				G: pixels[offset+1],
				B: pixels[offset+2],
				A: 255,
			})
			offset = offset + 3
		}
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {

		return err
	}
	return nil
}
