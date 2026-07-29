package cmd

import (
	"fmt"
	"os"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// SaveQRCode 生成二维码 PNG 图片
func SaveQRCode(url, outputPath string) error {
	dir := outputPath[:len(outputPath)-len("/bilibili_qrcode.png")]
	os.MkdirAll(dir, 0755)
	img, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, img, 0644)
}

// PrintQRCodeTerminal 在终端中打印二维码
func PrintQRCodeTerminal(url string) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return
	}
	qr.DisableBorder = true
	terminalStr := qr.ToSmallString(false)
	fmt.Fprintf(os.Stderr, "\n")
	for _, line := range strings.Split(terminalStr, "\n") {
		fmt.Fprintf(os.Stderr, "  %s\n", line)
	}
	fmt.Fprintf(os.Stderr, "  ⚡ 请用 B站 APP 扫描上方二维码登录\n")
	fmt.Fprintf(os.Stderr, "\n")
}
