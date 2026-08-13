#!/usr/bin/env python3
"""End-to-end smoke test of the Hairpin loop against a running Hairpin server.

Usage:
    go run ./cmd/hairpin &          # from server/, ideally with HAIRPIN_TIME_SCALE=60
    python3 scripts/smoke.py [device-id]

Set HAIRPIN_URL to point at a server other than http://localhost:8080.
Standard library only — no pip install.
"""
import json, os, sys, time, urllib.request

BASE = os.environ.get("HAIRPIN_URL", "http://localhost:8080")
TOKEN = None

def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if TOKEN:
        req.add_header("Authorization", "Bearer " + TOKEN)
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, json.load(r)
    except urllib.error.HTTPError as e:
        return e.code, json.load(e)

device = sys.argv[1] if len(sys.argv) > 1 else "smoke-device-0123456789"
_, auth = call("POST", "/v1/auth/device", {"device_id": device})
TOKEN = auth["token"]

_, s = call("GET", "/v1/state")
p, car = s["player"], s["build"]["car"]
print("=== driver ===")
print("  %s | rank %d | cash %d | rubies %d | rating %.1f"
      % (p["display_name"], p["ladder_rank"], p["cash"], p["rubies"], s["rating"]))
print("  car: %.0fhp %.0fkg (%.0f hp/t) grip %.0f handling %.0f braking %.0f"
      % (car["power"], car["weight"], car["power_to_wt"], car["grip"], car["handling"], car["braking"]))
d = s["build"]["driver"]
print("  driver stats: " + "  ".join("%s %.0f" % (k[:4], v) for k, v in d.items()))

print("\n=== jobs ===")
for j in s["job_board"]:
    lock = "LOCKED lvl %d" % j["unlocks_at_level"] if j["locked"] else "%ds" % j["duration_seconds"]
    print("  T%d %-28s %-12s $%-6d %s" % (j["tier"], j["name"], lock, j["cash"], j["flavour"][:44]))

code, r = call("POST", "/v1/jobs/start", {"job_id": "tofu_run"})
if code == 200:
    print("  -> started, %ds remaining" % r["active"]["remaining_seconds"])
    code, r = call("POST", "/v1/jobs/claim")
    print("  -> early claim rejected: %s (%d)" % (r.get("error"), code))
    time.sleep(2)
    code, r = call("POST", "/v1/jobs/claim")
    print("  -> claimed: +$%d, +%d xp, total $%d" % (r["cash"], r["xp"], r["total_cash"]))
    if r.get("drop"):
        print("  -> DROP: %s" % r["drop"]["name"])

print("\n=== garage ===")
_, g = call("GET", "/v1/garage")
for u in g["upgrades"]:
    print("  %-16s tier %d/%d  %-28s next $%-6s (%+.2f rating)"
          % (u["label"], u["tier"], u["max_tier"], u["tier_name"],
             u.get("next_cost", "-"), u.get("rating_delta", 0)))
code, r = call("POST", "/v1/garage/upgrade", {"category": "tires"})
print("  -> bought tires: paid $%d, rating %.2f -> %.2f" % (r["paid"], g["rating"], r["rating"]))
code, r = call("POST", "/v1/garage/upgrade", {"category": "tires", "cash": 99999999})
print("  -> injected 'cash' field rejected: %s (%d)" % (r.get("error"), code))

print("\n=== race ===")
_, o = call("GET", "/v1/race/opponent")
op = o["opponent"]
print("  next up: %s (rank %d, lvl %d, %s, rating %.1f) vs you %.1f"
      % (op["name"], op["ladder_rank"], op["driver_level"], op["chassis_name"],
         op["rating"], o["your_rating"]))
if op.get("notable_items"):
    print("  carrying: %s" % ", ".join(op["notable_items"]))

code, race = call("POST", "/v1/race/start")
rep = race["replay"]
print("\n  --- %s ---" % rep["track_name"])
print("  %-26s %8s %8s %9s" % ("SECTOR", "YOU", "RIVAL", "GAP"))
for seg in rep["segments"]:
    flag = ""
    if seg["challenger_mistake"]:
        flag += " [you: mistake]"
    if seg["opponent_mistake"]:
        flag += " [rival: mistake]"
    print("  %-26s %8.2f %8.2f %+9.2f%s"
          % (seg["name"], seg["challenger_time"], seg["opponent_time"], seg["gap"], flag))
print("  %-26s %8.2f %8.2f %+9.2f" % ("TOTAL", rep["challenger_total"], rep["opponent_total"],
                                       rep["opponent_total"] - rep["challenger_total"]))
print("\n  %s by %.2fs  (lead changes: %d%s)"
      % ("WIN" if race["won"] else "LOSS", rep["margin"], rep["lead_changes"],
         ", UPSET" if rep["upset"] else ""))
print("  reward: +$%d, +%d xp | rank %d -> %d | free races left %d"
      % (race["reward"]["cash"], race["reward"]["xp"], race["rank_before"],
         race["rank_after"], race["free_races_left"]))
if race.get("drop"):
    print("  DROP: %s (%s, quality %.2f)"
          % (race["drop"]["name"], race["drop"]["rarity"], race["drop"]["quality"]))

print("\n=== ladder (top 8) ===")
_, l = call("GET", "/v1/ladder?from=1&limit=8")
for e in l["entries"]:
    print("  %3d  %-34s lvl %-3d %-22s rating %6.1f  %d-%d"
          % (e["rank"], e["name"], e["driver_level"], e["chassis_name"],
             e["rating"], e["wins"], e["losses"]))

print("\n=== K's Office ===")
_, shop = call("GET", "/v1/shop")
for off in shop["offers"]:
    price = "$%d" % off["price_cash"] if off.get("price_cash") else "%d rubies" % off["price_rubies"]
    print("  %-28s %-10s %-7s %-12s q%.2f" % (off["name"], off["rarity"], off["slot"], price, off["quality"]))
