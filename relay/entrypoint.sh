#!/bin/sh
set -e

# Build nginx stream config from env — nginx itself reads SNI from the TLS
# ClientHello and connects to SNI_HOST:443 through gluetun's VPN namespace.
cat > /etc/nginx/nginx.conf <<EOF
events {}

stream {
    server {
        listen ${RELAY_PORT};
        ssl_preread on;

        # DNS resolver inside gluetun's VPN namespace
        resolver 8.8.8.8 1.1.1.1 valid=30s ipv6=off;

        proxy_pass \$ssl_preread_server_name:443;
        proxy_connect_timeout 10s;
        proxy_timeout 3600s;
        proxy_buffer_size 16k;
    }
}
EOF

exec nginx -g 'daemon off;'
