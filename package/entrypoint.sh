#!/bin/bash
set -e

if [ -x "$(command -v c_rehash)" ]; then
  # c_rehash is run here instead of update-ca-certificates because the latter requires root privileges
  # and the gke-operator container is run as non-root user.
  c_rehash
fi
gke-operator