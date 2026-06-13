package console

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mattn/go-tty"
)

func Input(m string) (string, error) {
	fmt.Print(m)

	reader := bufio.NewReader(os.Stdin)
	r, err := reader.ReadString('\n')
	return strings.TrimSpace(r), err
}

func getClipboard() (string, error) {
	cmd := exec.Command("pbpaste")

	o, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(o)), nil
}

func LongInput(m string) (string, error) {
	if runtime.GOOS != "darwin" {
		return Input(m)
	}

	fmt.Print(("(Press enter when you have it copied) ") + m)

	tty, err := tty.Open()
	if err != nil {
		return "", err
	}
	defer tty.Close()

	for {
		r, err := tty.ReadRune()
		if err != nil {
			return "", err
		}

		if r == '\r' || r == '\n' {
			break
		}
	}

	return getClipboard()
}
