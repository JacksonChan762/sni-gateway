#!/bin/sh
set -e

# Build nginx stream config from env. The relay runs inside gluetun's network
# namespace and uses gluetun's local DNS to avoid public-DNS round trips.
cat > /etc/nginx/nginx.conf <<EOF
worker_processes auto;
error_log /dev/stderr warn;

events {
    worker_connections 4096;
    multi_accept on;
    use epoll;
}

stream {
    access_log off;
    resolver ${RELAY_RESOLVER:-127.0.0.1} valid=${RELAY_DNS_VALID:-300s} ipv6=off;
    resolver_timeout ${RELAY_RESOLVER_TIMEOUT:-2s};

    server {
        listen ${RELAY_PORT} reuseport;
        ssl_preread on;
        preread_buffer_size 4k;

        proxy_pass \$ssl_preread_server_name:443;
        proxy_connect_timeout ${RELAY_CONNECT_TIMEOUT:-5s};
        proxy_timeout 3600s;
        proxy_buffer_size 16k;
        proxy_socket_keepalive on;
        tcp_nodelay on;
    }
}
EOF

exec nginx -g 'daemon off;'
