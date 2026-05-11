//go:build !windows

package winproc

import "os/exec"

func HideWindow(cmd *exec.Cmd) {}
