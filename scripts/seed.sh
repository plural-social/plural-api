#!/usr/bin/env bash
# Fills a development database with test users, image posts and follows.
# Safe to run twice: existing users are reused. Posts are added on every run.
set -euo pipefail

API=${API:-http://localhost:4100}
PASSWORD=password123
USERS=(alice bob carol)
POSTS_PER_USER=4

for tool in curl jq ffmpeg; do
  command -v "$tool" >/dev/null || { echo "seed: '$tool' is required but not installed" >&2; exit 1; }
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Tokens by username, kept in files because bash 3 (macOS) has no associative arrays.
for user in "${USERS[@]}"; do
  # 409 means the user already exists from an earlier run, which is fine.
  curl -s -o /dev/null -X POST "$API/accounts" \
    -d "{\"username\":\"$user\",\"email\":\"$user@example.com\",\"password\":\"$PASSWORD\"}"
  curl -sf -X POST "$API/sessions" -d "{\"login\":\"$user\",\"password\":\"$PASSWORD\"}" |
    jq -r .token >"$tmp/$user.token"
done

token() { cat "$tmp/$1.token"; }

posts=0
for user in "${USERS[@]}"; do
  for i in $(seq 1 "$POSTS_PER_USER"); do
    image="$tmp/$user-$i.jpg"
    ffmpeg -loglevel error -y -f lavfi -i "testsrc2=size=1080x1920:duration=1" \
      -vf "hue=h=$((RANDOM % 360))" -frames:v 1 "$image"
    media=$(curl -sf -X POST "$API/media" -H "Authorization: Bearer $(token "$user")" \
      -H 'Content-Type: image/jpeg' --data-binary "@$image" | jq -r .uuid)
    curl -sf -o /dev/null -X POST "$API/posts" -H "Authorization: Bearer $(token "$user")" \
      -d "{\"mediaUuid\":\"$media\",\"caption\":\"$user test post $i\"}"
    posts=$((posts + 1))
  done
done

follow() {
  curl -sf -o /dev/null -X PUT "$API/users/$2/follow" -H "Authorization: Bearer $(token "$1")"
}
follow alice bob
follow bob alice
follow carol alice

echo "Seeded ${#USERS[@]} users (${USERS[*]}), password '$PASSWORD', $posts new posts."
