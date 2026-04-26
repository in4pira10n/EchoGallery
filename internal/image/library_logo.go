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

// SaveSquareLibraryLogo 将输入图片完整缩放到正方形画布中，并输出为 PNG。
func SaveSquareLibraryLogo(src io.ReadSeeker, mimeType, destPath string, edge int) error {
	if edge <= 0 {
		edge = DefaultLibraryLogoEdge
	}
	img, err := decodeImage(src, mimeType)
	if err != nil {
		return fmt.Errorf("解码资源库图像失败: %w", err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, edge, edge))
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return fmt.Errorf("资源库图像尺寸无效")
	}
	scale := float64(edge) / float64(srcW)
	if hScale := float64(edge) / float64(srcH); hScale < scale {
		scale = hScale
	}
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	dstRect := image.Rect((edge-dstW)/2, (edge-dstH)/2, (edge-dstW)/2+dstW, (edge-dstH)/2+dstH)
	draw.CatmullRom.Scale(dst, dstRect, img, srcBounds, draw.Over, nil)

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
