#!/usr/bin/env bash
# Fills a development database with test users, image posts and follows.
# Safe to run twice: existing users are reused. Posts are added on every run.
set -euo pipefail

API=${API:-http://localhost:4100}
PASSWORD=password123
USERS=(alice bob carol)
POSTS_PER_USER=4
VIDEOS_PER_USER=2

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

# Videos stay "processing" until the transcoder (make run-transcoder) handles them.
for user in "${USERS[@]}"; do
  for i in $(seq 1 "$VIDEOS_PER_USER"); do
    video="$tmp/$user-v$i.mp4"
    ffmpeg -loglevel error -y \
      -f lavfi -i "testsrc2=size=1080x1920:rate=30:duration=6" \
      -f lavfi -i "sine=frequency=$((300 + RANDOM % 500)):duration=6" \
      -c:v libx264 -pix_fmt yuv420p -c:a aac -shortest "$video"
    media=$(curl -sf -X POST "$API/media" -H "Authorization: Bearer $(token "$user")" \
      -H 'Content-Type: video/mp4' --data-binary "@$video" | jq -r .uuid)
    curl -sf -o /dev/null -X POST "$API/posts" -H "Authorization: Bearer $(token "$user")" \
      -d "{\"mediaUuid\":\"$media\",\"caption\":\"$user test video $i\"}"
    posts=$((posts + 1))
  done
done

follow() {
  curl -sf -o /dev/null -X PUT "$API/users/$2/follow" -H "Authorization: Bearer $(token "$1")"
}
follow alice bob
follow bob alice
follow carol alice

# Each user likes every post by the people they follow. Likes are idempotent.
like_posts_of() {
  local liker=$1 author=$2
  curl -sf "$API/users/$author/posts?limit=30" | jq -r '.items[].uuid' | while read -r post; do
    curl -sf -o /dev/null -X PUT "$API/posts/$post/like" -H "Authorization: Bearer $(token "$liker")"
  done
}
like_posts_of alice bob
like_posts_of bob alice
like_posts_of carol alice

# Comments are not idempotent: every run adds another one.
first_alice_post=$(curl -sf "$API/users/alice/posts?limit=30" | jq -r '.items[-1].uuid')
curl -sf -o /dev/null -X POST "$API/posts/$first_alice_post/comments" \
  -H "Authorization: Bearer $(token bob)" -d '{"body":"nice one"}'

echo "Seeded ${#USERS[@]} users (${USERS[*]}), password '$PASSWORD', $posts new posts."
echo "Video posts appear in feeds once the transcoder has processed them (make run-transcoder)."
