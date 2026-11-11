// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"context"
	"os/exec"
	"time"
)

// runTimeout runs a command with a deadline, returning its output.
func runTimeout(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}
