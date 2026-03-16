AUTH="$(cat /app/config/redis-auth/auth)"
if [ -z "${AUTH}" ]; then
    echo "Error: Redis password not mounted correctly"
    exit 1
fi
response=$(
  env REDISCLI_AUTH="${AUTH}" redis-cli \
    -h localhost \
    -p 6379 \
{{- if eq .UseTLS "true"}}
    --tls \
    --cacert /app/config/redis/tls/tls.crt \
{{- end}}
    ping
)
if [ "$response" != "PONG" ] ; then
  echo "$response"
  exit 1
fi
echo "response=$response"
