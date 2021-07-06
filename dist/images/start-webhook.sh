#!/usr/bin/env bash
set -euo pipefail
export OVN_NB_DAEMON=$(ovn-nbctl --db=tcp:${OVN_OVSDB_NB_SERVICE_HOST}:${OVN_OVSDB_NB_SERVICE_PORT} --pidfile --detach)
exec ./kube-ovn-webhook --ovn-nb-host=${OVN_OVSDB_NB_SERVICE_HOST} --ovn-nb-port=${OVN_OVSDB_NB_SERVICE_PORT} $@