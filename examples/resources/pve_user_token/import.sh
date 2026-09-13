#!/bin/sh
# Import a PVE API token as <userid>!<tokenid>.
terraform import pve_user_token.ci "root@pam!ci"
