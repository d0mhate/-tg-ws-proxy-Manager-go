package mtprotoproxy

import (
	"bytes"

	qrterminal "github.com/mdp/qrterminal/v3"
)

func RenderTerminalQR(text string) (string, error) {
	var buf bytes.Buffer
	qrterminal.GenerateHalfBlock(text, qrterminal.M, &buf)
	return buf.String(), nil
}
