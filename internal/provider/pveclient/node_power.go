// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
)

// Node power commands accepted by POST /nodes/{node}/status (upstream
// `node_cmd`); the pin defines no further parameters for this endpoint.
const (
	NodePowerCommandReboot   = "reboot"
	NodePowerCommandShutdown = "shutdown"
)

// NodeReboot issues POST /nodes/{node}/status with `command: reboot`. The
// endpoint is synchronous: PVE returns null once the reboot has been
// initiated, so there is no task UPID to wait for.
func (c *Client) NodeReboot(ctx context.Context, node string) error {
	return c.nodeCommand(ctx, node, NodePowerCommandReboot)
}

// NodeShutdown issues POST /nodes/{node}/status with `command: shutdown`.
// Like the reboot command it returns null without a task UPID.
func (c *Client) NodeShutdown(ctx context.Context, node string) error {
	return c.nodeCommand(ctx, node, NodePowerCommandShutdown)
}

// nodeCommand posts the shared node_cmd request; PVE proxies it to the
// target node and requires the Sys.PowerMgmt privilege on /nodes/{node}.
func (c *Client) nodeCommand(ctx context.Context, node, command string) error {
	path := fmt.Sprintf("/nodes/%s/status", node)
	body := map[string]string{"command": command}
	return c.Do(ctx, "POST", path, body, nil)
}
