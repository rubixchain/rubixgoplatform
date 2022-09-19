package util

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

func GetPNGImagePixels(file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y
	pixels := make([]byte, 0)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			pixels = append(pixels, byte(r>>8))
			pixels = append(pixels, byte(g>>8))
			pixels = append(pixels, byte(b>>8))
		}
	}
	return pixels, nil
}
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
