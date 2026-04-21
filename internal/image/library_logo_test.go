package image

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestSaveSquareLibraryLogo_CropsCenterSquare(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	src.Set(1, 0, color.RGBA{G: 255, A: 255})
	src.Set(2, 0, color.RGBA{B: 255, A: 255})
	src.Set(3, 0, color.RGBA{R: 255, G: 255, A: 255})
	src.Set(0, 1, color.RGBA{R: 255, A: 255})
	src.Set(1, 1, color.RGBA{G: 255, A: 255})
	src.Set(2, 1, color.RGBA{B: 255, A: 255})
	src.Set(3, 1, color.RGBA{R: 255, G: 255, A: 255})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatalf("编码测试图片失败: %v", err)
	}

	destPath := t.TempDir() + "/library-logo.png"
	if err := SaveSquareLibraryLogo(bytes.NewReader(encoded.Bytes()), "image/png", destPath, 2); err != nil {
		t.Fatalf("保存资源库图像失败: %v", err)
	}

	output := mustOpenFile(t, destPath)
	defer output.Close()
	decoded, err := png.Decode(output)
	if err != nil {
		t.Fatalf("解码输出图片失败: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Fatalf("期望输出为 2x2，得到 %dx%d", bounds.Dx(), bounds.Dy())
	}
	if !colorEqual(decoded.At(0, 0), color.RGBA{G: 255, A: 255}) {
		t.Fatalf("期望左侧来自中心裁剪后的绿色区域")
	}
	if !colorEqual(decoded.At(1, 0), color.RGBA{B: 255, A: 255}) {
		t.Fatalf("期望右侧来自中心裁剪后的蓝色区域")
	}
}
