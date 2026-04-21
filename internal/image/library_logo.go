package image

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

const DefaultLibraryLogoEdge = 512

// SaveSquareLibraryLogo 将输入图片按中心裁剪为正方形，并输出为 PNG。
func SaveSquareLibraryLogo(src io.ReadSeeker, mimeType, destPath string, edge int) error {
	if edge <= 0 {
		edge = DefaultLibraryLogoEdge
	}
	img, err := decodeImage(src, mimeType)
	if err != nil {
		return fmt.Errorf("解码资源库图像失败: %w", err)
	}
	cropped := cropCenterSquare(img)
	dst := image.NewRGBA(image.Rect(0, 0, edge, edge))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建资源库图像目录失败: %w", err)
	}
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建资源库图像文件失败: %w", err)
	}
	defer file.Close()
	if err := png.Encode(file, dst); err != nil {
		return fmt.Errorf("写入资源库图像失败: %w", err)
	}
	return nil
}

func cropCenterSquare(src image.Image) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	size := width
	if height < size {
		size = height
	}
	offsetX := bounds.Min.X + (width-size)/2
	offsetY := bounds.Min.Y + (height-size)/2
	rect := image.Rect(0, 0, size, size)
	dst := image.NewRGBA(rect)
	draw.Draw(dst, rect, src, image.Point{X: offsetX, Y: offsetY}, draw.Src)
	return dst
}
