FROM nginx:1.25-alpine

# gettext provides envsubst for config templating
RUN apk add --no-cache gettext

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# RELAY_PORT is set by the backend when creating each exit container
ENV RELAY_PORT=10001

ENTRYPOINT ["/entrypoint.sh"]
