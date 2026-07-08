package command

import (
	"os"

	qrcode "github.com/skip2/go-qrcode"
)

func SaveQRCode(url, outputPath string) error {
	dir := outputPath[:len(outputPath)-len("/bilibili_qrcode.png")]
	os.MkdirAll(dir, 0755)

	img, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, img, 0644)
}
