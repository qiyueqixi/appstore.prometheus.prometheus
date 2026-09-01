package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const canvasSize = 2048

var (
	transparent = color.RGBA{}
	tileColor   = color.RGBA{R: 18, G: 39, B: 44, A: 255}
	brandColor  = color.RGBA{R: 237, G: 76, B: 37, A: 255}
	white       = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	source := renderIcon()
	icon64 := downsample(source, 64)
	icon256 := downsample(source, 256)

	outputs := map[string]image.Image{
		"app/ui/images/icon_64.png":  icon64,
		"app/ui/images/icon_256.png": icon256,
		"app/ui/images/ICON.PNG":     icon64,
		"prometheus/ICON.PNG":        icon64,
		"prometheus/ICON_256.PNG":    icon256,
	}
	for name, icon := range outputs {
		if err := writePNG(filepath.Join(*root, filepath.FromSlash(name)), icon); err != nil {
			panic(err)
		}
	}
}

func renderIcon() *image.RGBA {
	icon := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	fill(icon, transparent)
	fillRoundedRect(icon, point(4, 4), point(252, 252), unit(48), tileColor)
	fillCircle(icon, point(128, 116), unit(98), brandColor)

	strokeCubic(icon,
		point(75, 112),
		point(75, 174),
		point(181, 174),
		point(181, 112),
		unit(14),
		white,
	)
	strokeLine(icon, point(128, 116), point(168, 57), unit(16), white)
	strokeLine(icon, point(92, 220), point(164, 220), unit(15), white)
	return icon
}

func point(x, y float64) image.Point {
	return image.Pt(int(math.Round(x*canvasSize/256)), int(math.Round(y*canvasSize/256)))
}

func unit(value float64) int {
	return int(math.Round(value * canvasSize / 256))
}

func fill(icon *image.RGBA, value color.RGBA) {
	for y := icon.Bounds().Min.Y; y < icon.Bounds().Max.Y; y++ {
		for x := icon.Bounds().Min.X; x < icon.Bounds().Max.X; x++ {
			icon.SetRGBA(x, y, value)
		}
	}
}

func fillRoundedRect(icon *image.RGBA, min, max image.Point, radius int, value color.RGBA) {
	leftCenter := min.X + radius
	rightCenter := max.X - radius
	topCenter := min.Y + radius
	bottomCenter := max.Y - radius
	radiusSquared := radius * radius
	for y := min.Y; y < max.Y; y++ {
		for x := min.X; x < max.X; x++ {
			nearestX := clamp(x, leftCenter, rightCenter)
			nearestY := clamp(y, topCenter, bottomCenter)
			deltaX := x - nearestX
			deltaY := y - nearestY
			if deltaX*deltaX+deltaY*deltaY <= radiusSquared {
				icon.SetRGBA(x, y, value)
			}
		}
	}
}

func fillCircle(icon *image.RGBA, center image.Point, radius int, value color.RGBA) {
	radiusSquared := radius * radius
	for y := center.Y - radius; y <= center.Y+radius; y++ {
		for x := center.X - radius; x <= center.X+radius; x++ {
			deltaX := x - center.X
			deltaY := y - center.Y
			if deltaX*deltaX+deltaY*deltaY <= radiusSquared {
				icon.SetRGBA(x, y, value)
			}
		}
	}
}

func strokeLine(icon *image.RGBA, start, end image.Point, width int, value color.RGBA) {
	deltaX := float64(end.X - start.X)
	deltaY := float64(end.Y - start.Y)
	steps := int(math.Ceil(math.Hypot(deltaX, deltaY)))
	radius := width / 2
	for step := 0; step <= steps; step++ {
		ratio := float64(step) / float64(steps)
		center := image.Pt(
			int(math.Round(float64(start.X)+deltaX*ratio)),
			int(math.Round(float64(start.Y)+deltaY*ratio)),
		)
		fillCircle(icon, center, radius, value)
	}
}

func strokeCubic(icon *image.RGBA, start, controlA, controlB, end image.Point, width int, value color.RGBA) {
	previous := start
	for step := 1; step <= 320; step++ {
		t := float64(step) / 320
		inverse := 1 - t
		current := image.Pt(
			int(math.Round(inverse*inverse*inverse*float64(start.X)+3*inverse*inverse*t*float64(controlA.X)+3*inverse*t*t*float64(controlB.X)+t*t*t*float64(end.X))),
			int(math.Round(inverse*inverse*inverse*float64(start.Y)+3*inverse*inverse*t*float64(controlA.Y)+3*inverse*t*t*float64(controlB.Y)+t*t*t*float64(end.Y))),
		)
		strokeLine(icon, previous, current, width, value)
		previous = current
	}
}

func downsample(source *image.RGBA, size int) *image.RGBA {
	destination := image.NewRGBA(image.Rect(0, 0, size, size))
	factor := source.Bounds().Dx() / size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var alpha, red, green, blue uint64
			for sourceY := y * factor; sourceY < (y+1)*factor; sourceY++ {
				for sourceX := x * factor; sourceX < (x+1)*factor; sourceX++ {
					pixel := source.RGBAAt(sourceX, sourceY)
					alpha += uint64(pixel.A)
					red += uint64(pixel.R) * uint64(pixel.A)
					green += uint64(pixel.G) * uint64(pixel.A)
					blue += uint64(pixel.B) * uint64(pixel.A)
				}
			}
			count := uint64(factor * factor)
			if alpha == 0 {
				continue
			}
			destination.SetRGBA(x, y, color.RGBA{
				R: uint8(red / alpha),
				G: uint8(green / alpha),
				B: uint8(blue / alpha),
				A: uint8(alpha / count),
			})
		}
	}
	return destination
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func writePNG(path string, icon image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, icon)
}
