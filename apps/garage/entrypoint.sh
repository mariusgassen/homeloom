#!/bin/sh
# Starts Garage and sets it up, every time and idempotently: a single-node layout, the bucket
# S3_BUCKET, and the key S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY with access to it. Without a
# key the store runs empty and the API keeps photos in Postgres.
set -eu

meta=/var/lib/garage/meta
mkdir -p "$meta" /var/lib/garage/data
# Only this node uses the RPC secret, so it is made once and kept in the volume.
if [ ! -s "$meta/rpc_secret" ]; then
	(umask 077 && od -An -N32 -tx1 /dev/urandom | tr -d ' \n' >"$meta/rpc_secret")
fi

garage server &
pid=$!
trap 'kill -TERM "$pid"; wait "$pid"; exit' TERM INT

i=0
until garage status >/dev/null 2>&1; do
	i=$((i + 1))
	if [ "$i" -ge 60 ] || ! kill -0 "$pid" 2>/dev/null; then
		echo "garage did not start" >&2
		exit 1
	fi
	sleep 1
done

setup() {
	# Capacity only weighs nodes against each other; with one node any value works.
	if [ ! -e "$meta/homeloom-layout" ]; then
		node=$(garage node id -q | cut -d@ -f1)
		garage layout assign -z homeloom -c 1T "$node"
		garage layout apply --version 1
		touch "$meta/homeloom-layout"
	fi

	bucket=${S3_BUCKET:-homeloom}
	garage bucket info "$bucket" >/dev/null 2>&1 || garage bucket create "$bucket"

	id=${S3_ACCESS_KEY_ID:-}
	secret=${S3_SECRET_ACCESS_KEY:-}
	if [ -z "$id" ] || [ -z "$secret" ]; then
		echo "S3_ACCESS_KEY_ID or S3_SECRET_ACCESS_KEY not set: no key, photos stay in Postgres" >&2
		return 0
	fi
	if garage key info "$id" >/dev/null 2>&1; then
		# Garage never reuses a key id, so a new secret needs a new id as well.
		if ! garage key info --show-secret "$id" | grep -qF "$secret"; then
			echo "S3_SECRET_ACCESS_KEY changed for $id: set a new S3_ACCESS_KEY_ID with it" >&2
			return 1
		fi
	else
		garage key import --yes -n homeloom "$id" "$secret"
	fi
	garage bucket allow --read --write --owner "$bucket" --key "$id"
}

if ! setup >/dev/null; then
	echo "garage setup failed" >&2
	kill -TERM "$pid"
	exit 1
fi
echo "garage ready"
wait "$pid"
