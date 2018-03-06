#!/usr/bin/env bash
[[ -f examples/proxy-protocol/haproxy.pid ]] && rm examples/proxy-protocol/haproxy.pid
fail() { echo "$*"; exit 1; }

set -x
which haproxy || fail haproxy is missing
which socat   || fail socat is missing

test -f goircd || fail goircd is missing

haproxy_cfg() {
    cat<<__cfg
global
    daemon
    maxconn 4

defaults
    mode tcp
    timeout server  3600
    timeout client  3600
    timeout connect 5

listen ircd-demo
    bind *:9667
    server goircd-pv2    127.0.0.1:6667 $1
__cfg
}

./goircd &
trap "kill $!" EXIT 

# direct connect
haproxy_cfg > examples/proxy-protocol/haproxy.cfg
haproxy -f examples/proxy-protocol/haproxy.cfg -c
haproxy -f examples/proxy-protocol/haproxy.cfg -p examples/proxy-protocol/haproxy.pid
(sleep 1; echo "CONNECT" ) | socat stdin tcp:127.0.0.1:9667
kill $(cat examples/proxy-protocol/haproxy.pid)


# proxy v1 protocol
haproxy_cfg send-proxy > examples/proxy-protocol/haproxy.cfg
haproxy -f examples/proxy-protocol/haproxy.cfg -c
haproxy -f examples/proxy-protocol/haproxy.cfg -p examples/proxy-protocol/haproxy.pid
(sleep 1; echo "CONNECT" ) | socat stdin tcp:127.0.0.1:9667
kill $(cat examples/proxy-protocol/haproxy.pid)

# proxy v2 protocol
haproxy_cfg send-proxy-v2 > examples/proxy-protocol/haproxy.cfg
haproxy -f examples/proxy-protocol/haproxy.cfg -c
haproxy -f examples/proxy-protocol/haproxy.cfg -p examples/proxy-protocol/haproxy.pid
(sleep 1; echo "CONNECT" ) | socat stdin tcp:127.0.0.1:9667
kill $(cat examples/proxy-protocol/haproxy.pid)

rm examples/proxy-protocol/haproxy.pid