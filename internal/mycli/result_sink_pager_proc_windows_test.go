//go:build windows

package mycli

import "os/exec"

func configurePagerSubprocess(cmd *exec.Cmd) {}

func killPagerSubprocess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
