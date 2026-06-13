package console

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
)

func ClearScreen() {
	if config.GetBool("verbose") {
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "cls")
	default:
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}
