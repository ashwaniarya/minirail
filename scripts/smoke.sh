#!/bin/sh
# Needs CONSOLE_URL, ADMIN_PASSWORD, APPS_DOMAIN (make smoke fills these) and SMOKE_REPO,
# the https://github.com/<owner>/<repo> URL of this repository.
set -eu
: "${CONSOLE_URL:?}" "${ADMIN_PASSWORD:?}" "${APPS_DOMAIN:?}" "${SMOKE_REPO:?}"

app_url="https://hello.$APPS_DOMAIN"

console() {
	curl -fsS -u "admin:$ADMIN_PASSWORD" -o /dev/null "$@"
}

wait_for() {
	deadline=$(($(date +%s) + 900))
	until curl -fsS "$1" 2>/dev/null | grep -q "$2"; do
		if [ "$(date +%s)" -ge "$deadline" ]; then
			echo "FAIL: $1 never returned '$2'; the app is left in place for debugging" >&2
			exit 1
		fi
		sleep 10
	done
	echo "OK: $1 returned '$2'"
}

console -d name=hello -d repo="$SMOKE_REPO" -d branch=main -d root=examples/hello \
	-d type=web -d port=8080 -d replicas=1 "$CONSOLE_URL/apps/new"
wait_for "$app_url/" ok

console -X POST "$CONSOLE_URL/apps/hello/attach/postgres"
wait_for "$app_url/db" "DATABASE_URL set"

console -X POST "$CONSOLE_URL/apps/hello/delete"
echo "OK: smoke passed"
