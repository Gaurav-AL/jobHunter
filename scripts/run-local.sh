#!/usr/bin/env sh
set -eu

if [ ! -f .env ]; then
  printf '%s\n' "No .env found. Creating one from .env.example; fill Firebase values for login/register to work."
  cp .env.example .env
fi

docker compose up --build
