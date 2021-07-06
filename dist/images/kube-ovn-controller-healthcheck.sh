#!/bin/bash
set -euo pipefail

if [[ -z "${NO_OVN}" ]]; then
  OVN_NB_DAEMON=/var/run/ovn/ovn-nbctl.$(cat /var/run/ovn/ovn-nbctl.pid).ctl ovn-nbctl --timeout=15 lr-list > /dev/null
fi

nc -z -w3 127.0.0.1 10660

kubectl version
