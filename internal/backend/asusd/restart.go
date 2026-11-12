// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"context"
	"fmt"
	"os/exec"
)

// Restart implements backend.Backend with logind's normal reboot path.
// The queued MUX value is applied by asusd/asus-shutdown during the
// shutdown that precedes the reboot, so this is the Linux equivalent of
// G-Helper's `shutdown /r`.
func (b *Backend) Restart(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "systemctl", "reboot").Run(); err != nil {
		return fmt.Errorf("asusd: reboot: %w", err)
	}
	return nil
}
