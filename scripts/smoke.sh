#!/usr/bin/env bash
# 上线冒烟检查：在部署后的环境上验证「可登录 → 可见时段 → 可提交预约 → 名额被扣减」。
# 用法：scripts/smoke.sh [gateway_base_url]
# 前置：docker compose 已起 mysql/service/gateway/paysim，且已执行过 seed。
set -uo pipefail

GW="${1:-http://127.0.0.1:8081}"
PASS="${MYSQL_ROOT_PASSWORD:-petshop123}"
COOKIE_OWNER=$(mktemp)
COOKIE_USER=$(mktemp)
FAILED=0

pass() { echo "SMOKE OK   $*"; }
fail() { echo "SMOKE FAIL $*"; FAILED=1; }

# 1) 健康检查：网关存活
code=$(curl -s -o /dev/null -w '%{http_code}' "$GW/login")
[ "$code" = "200" ] && pass "GET /login -> 200" || fail "GET /login -> $code"

# 2) 店长登录并发布时段（写操作，验证 RPC 与 DB 链路）
curl -s -c "$COOKIE_OWNER" -o /dev/null -d 'username=owner' "$GW/login"
START=$(date -d '+1 day 10:00' '+%Y-%m-%d %H:%M')
END=$(date -d '+1 day 11:00' '+%Y-%m-%d %H:%M')
curl -s -b "$COOKIE_OWNER" -o /dev/null --data-urlencode "start_time=$START" --data-urlencode "end_time=$END" -d 'capacity=3' "$GW/owner/slots"
SLOT_ID=$(docker compose exec -T mysql mysql -uroot -p"$PASS" petshop -N -B \
  -e "SELECT id FROM time_slots ORDER BY id DESC LIMIT 1" 2>/dev/null | tr -d '\r')
[ -n "$SLOT_ID" ] && pass "店长发布时段 slot_id=$SLOT_ID" || fail "时段未落库"

# 3) 主人登录后能看到该时段
curl -s -c "$COOKIE_USER" -o /dev/null -d 'username=user1' "$GW/login"
curl -s -b "$COOKIE_USER" "$GW/slots" -o /tmp/slots.html
grep -q "时段" /tmp/slots.html && pass "GET /slots 渲染时段" || fail "GET /slots 未渲染时段"

# 4) 主人提交预约，名额应由 3 扣到 2
PET_ID=$(docker compose exec -T mysql mysql -uroot -p"$PASS" petshop -N -B \
  -e "SELECT id FROM pets WHERE user_id=(SELECT id FROM users WHERE username='user1') LIMIT 1" 2>/dev/null | tr -d '\r')
curl -s -b "$COOKIE_USER" -o /dev/null -d "slot_id=$SLOT_ID" -d "pet_id=$PET_ID" "$GW/bookings"
REMAIN=$(docker compose exec -T mysql mysql -uroot -p"$PASS" petshop -N -B \
  -e "SELECT remaining FROM time_slots WHERE id=$SLOT_ID" 2>/dev/null | tr -d '\r')
CNT=$(docker compose exec -T mysql mysql -uroot -p"$PASS" petshop -N -B \
  -e "SELECT COUNT(*) FROM appointments WHERE slot_id=$SLOT_ID" 2>/dev/null | tr -d '\r')
[ "$REMAIN" = "2" ] && pass "提交预约后名额=2" || fail "提交预约后名额=$REMAIN（期望 2）"
[ "$CNT" = "1" ] && pass "预约单已落库" || fail "预约单数量=$CNT（期望 1）"

# 5) 教学时钟可写可复位
curl -s -b "$COOKIE_OWNER" -o /dev/null -d 'offset_sec=7200' "$GW/clock"
OFFSET=$(docker compose exec -T mysql mysql -uroot -p"$PASS" petshop -N -B \
  -e "SELECT offset_sec FROM clock_offsets WHERE id=1" 2>/dev/null | tr -d '\r')
[ "$OFFSET" = "7200" ] && pass "教学时钟偏移已持久化" || fail "时钟偏移=$OFFSET（期望 7200）"
curl -s -b "$COOKIE_OWNER" -o /dev/null -d 'offset_sec=0' "$GW/clock"

rm -f "$COOKIE_OWNER" "$COOKIE_USER"
if [ "$FAILED" -ne 0 ]; then
  echo "冒烟失败：部署不可判定为成功"
  exit 1
fi
echo "冒烟全部通过：$GW"
