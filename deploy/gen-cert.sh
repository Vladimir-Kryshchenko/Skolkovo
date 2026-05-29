#!/usr/bin/env bash
# Генерация self-signed TLS-сертификата для «База Сколково» (домена нет — по IP).
set -euo pipefail

CERT_DIR="$(cd "$(dirname "$0")" && pwd)/certs"
IP="${1:-213.136.75.7}"

mkdir -p "$CERT_DIR"

if [ -f "$CERT_DIR/server.crt" ] && [ -f "$CERT_DIR/server.key" ]; then
  echo "[cert] сертификат уже существует ($CERT_DIR) — пропускаю"
  exit 0
fi

openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout "$CERT_DIR/server.key" \
  -out    "$CERT_DIR/server.crt" \
  -days   825 \
  -subj   "/C=RU/O=Baza Skolkovo (test)/CN=$IP" \
  -addext "subjectAltName=IP:$IP,DNS:localhost"

chmod 600 "$CERT_DIR/server.key"
echo "[cert] создан self-signed сертификат для $IP (825 дней) в $CERT_DIR"
