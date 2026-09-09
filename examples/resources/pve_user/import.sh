#!/bin/sh
# Copyright (c) HashiCorp, Inc.

# Import a PVE user by its full user ID.
terraform import pve_user.ci "root@pam"
