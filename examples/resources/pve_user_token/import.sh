#!/bin/sh
# Copyright (c) HashiCorp, Inc.

# Import a PVE API token as <userid>!<tokenid>.
terraform import pve_user_token.ci "root@pam!ci"
