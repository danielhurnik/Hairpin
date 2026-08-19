# Hairpin

An idle/progression racing game in the Shakes & Fidget mould, themed around
touge — Japanese mountain-pass street racing.

Work a job, spend the cash on your car, race someone else's saved build down a
mountain, climb the ladder. Milestone 1 is that loop, playable end to end.

---

## Stack

| Layer    | Choice                                              | Why |
|----------|-----------------------------------------------------|-----|
| Server   | **Go 1.24**, standard-library `net/http`             | The whole game is short transactional requests. Go starts in milliseconds, handles thousands of concurrent players per box, and deploys as one static binary. No framework needed — Go 1.22+ routing patterns cover the whole API. |
| Database | **SQLite** (`modernc.org/sqlite`, pure Go, no cgo)   | Zero setup for development and genuinely fine into the thousands of players. All SQL is in `internal/store` behind one interface, so moving to Postgres is a contained change. |
| Client   | **Expo SDK 57** / React Native / TypeScript          | One codebase for iOS and Android, over-the-air updates for balance tweaks, and Expo Go means testing on a real phone without a build step. |
| Art      | **8-bit sprites**, generated to SVG rects             | Cars are top-down pixel sprites defined as pixel rows, so the livery recolours them at runtime — seven chassis times seven liveries with no image assets to ship. |
| Routing  | **expo-router**                                      | File-based routes, typed, and the tab layout is declarative. |

Total server dependencies: **one** (the SQLite driver). Everything else is
standard library.

---

## The rule that shapes everything

**The client is a renderer. The server is the game.**

Every number a player could profit from lying about — cash, rubies, job
completion, race outcomes, ladder position, drop rolls, shop prices — is
computed and enforced inside a database transaction on the server. The client
sends intent (`start this job`, `race`) and receives results it can only
display.

Concretely:

- **Stats are never stored, always derived.** A build's power, grip and driver
  attributes are recomputed from chassis + upgrade tiers + equipped items on
  every read. There is no field a tampered client could write.
- **Timers are server clocks.** `POST /v1/jobs/claim` re-checks completion
  against the server's own clock. A partial unique index (`WHERE claimed_at IS
  NULL`) makes double-claiming a database error, not a race condition.
- **Prices come from server tables.** Purchase requests name a category or an
  offer ID; the amount is looked up server-side. Request bodies reject unknown
  fields, so `{"category":"tires","cash":999999}` is a 400.
- **Races are decided server-side and stored as a seed.** The client receives a
  replay to animate. It cannot influence, re-roll, or replay it for a different
  result.
- **Receipts are unique.** Replaying an in-app purchase is a constraint
  violation.

The test suite attacks these directly — see `TestEconomyCannotBeCheated`.

---

## Running it

### Quickest path — one command

```bash
./dev.sh web      # server + the game in your browser
./dev.sh          # server + Expo (phone, simulator, or web from the menu)
./dev.sh smoke    # server + a scripted run through the whole loop
```

`dev.sh` starts the server, waits for it to be healthy, works out your LAN IP so
Expo Go on a real phone can reach it, and compresses job timers 60x — a 4-hour
job finishes in 4 minutes, a 30-second one is instant. Set
`HAIRPIN_TIME_SCALE=1` for real time.

Requirements: Go 1.24+, Node 20+, and a phone with Expo Go if you want it on a
real device.

### Server

```bash
cd server
go run ./cmd/hairpin
```

Listens on `:8080` and seeds 120 ladder rivals on first boot.

Useful environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `HAIRPIN_ADDR` | `:8080` | Listen address |
| `HAIRPIN_DSN` | `file:hairpin.db?…` | SQLite DSN |
| `HAIRPIN_TOKEN_SECRET` | *random* | **Set this in production.** Signs session tokens; a random dev value drops all sessions on restart |
| `HAIRPIN_TIME_SCALE` | `1` | Divides every job timer. `60` turns a 4-hour job into 4 minutes — invaluable for testing |
| `HAIRPIN_SEED_BOTS` | `120` | Ladder rivals to create on first boot |
| `HAIRPIN_DEV_IAP` | `true` | **Set to `false` in production.** Grants rubies without a store receipt |
| `HAIRPIN_RATE_PER_SEC` / `HAIRPIN_RATE_BURST` | `5` / `20` | Per-player limit on mutating requests |

```bash
go test ./...                                        # everything
go test ./internal/game -run Calibration -v          # the race tuning curve
```

### Client

```bash
cd client
npm install
npx expo start
```

On a simulator, the defaults work. **On a physical device, `localhost` is the
phone** — point it at your machine:

```bash
EXPO_PUBLIC_API_URL=http://192.168.1.42:8080 npx expo start
```

### Smoke test

`scripts/smoke.py` drives the whole loop over HTTP against a running server and
prints a race, sector by sector:

```bash
HAIRPIN_TIME_SCALE=60 go run ./cmd/hairpin &
python3 scripts/smoke.py
```

---

## The race model

This is the part with a hard requirement attached: **the favourite should win
about 65% of the time.**

That number does most of the design work. A 65% favourite means a 35% upset
rate, which is a *lot* of variance — far more than real racing. It has to come
from somewhere deliberate, or it leaks in somewhere accidental.

### How a race resolves

A track is 10–12 pieces of road, each described by its geometry (see *The road
is described, not labelled* below) and classified from it into a kind —
`straight`, `hairpin`, `sweeper`, `technical`, `braking`, `chicane`. Each kind
weights car and driver attributes differently — a straight is 78% power, a hairpin is 38% handling and leans on
reflexes; a fast sweeper rewards bravery. Six tracks ship, from Akina's five
consecutive hairpins to Happogahara's high-speed sweepers.

For each segment, per racer:

1. **Performance** = weighted car stats (72%) + weighted driver stats (28%),
   adjusted by drivetrain and by stamina late in the run. A stock car with an
   average driver scores ~100.
2. **Base time** = `segment_seconds × (100 / performance)^0.55`. The exponent
   compresses the advantage — a 10% better car is not 10% faster down a
   mountain.
3. **Variance**, from three sources:
   - **Form** — a correlated per-run term (σ ≈ 4.7%). The dominant source of
     upsets.
   - **Segment noise** — per corner, scaled by that corner's risk and narrowed
     by the driver's consistency.
   - **Mistakes** — lockups and missed apexes, ~4% per corner. Probability rises
     with bravery and the corner's risk, falls with consistency and
     intelligence. Costs 8–26% of that segment.

Everything is a pure function of `(track, builds, seed)`, so the server stores
only the seed and can re-derive any race exactly — for replay, or for an audit.

### Calibration

`FormMomentum` deserves a note. Form was originally one draw per race, which
hit 65% correctly but produced a dead presentation: the run was decided at the
start line and the gap only ever grew. Form is now an AR(1) walk — a driver
finds a rhythm or loses one — rescaled so total variance is *unchanged*. Win
rates are identical; **59% of close races now change lead at least once.**

Measured over 200,000 races per row (`TestCalibrationCurve`):

| Rating edge | Favourite wins |
|---|---|
| +0%  | 49.9% |
| +2%  | 56.4% |
| **+5%**  | **65.3%** ← the design target |
| +10% | 78.0% |
| +15% | 87.3% |
| +25% | 96.5% |
| +50% | 99.9% |

The tuning constants sit at the top of `internal/game/race.go`. Change one and
re-run the calibration test to see what it did.

---

## Progression

**Jobs** — five tiers, 30 seconds to 4 hours, gated by driver level. One at a
time. Rubies skip the remaining wait at roughly one per ten minutes.

**Upgrades** — five categories (engine, turbo, tires, suspension, weight
reduction), five tiers each, ¥180 to ¥36,000. Each has a personality: turbo
buys the most power but adds variance (lag), weight reduction is cheap and
quiet. The garage shows the exact rating delta each purchase would buy,
computed with the same model that resolves races.

**Cars** — seven chassis, ¥12,000 to ¥160,000, gated by driver level as well as
by price. You own them rather than trade them in: switching between owned cars
is free, and each car keeps its own upgrade tiers.

That last part is the whole design. A newly bought chassis arrives **stock**,
so it is usually *slower* than the car you have been building for hours — the
showroom says so out loud rather than hiding it. A full rebuild is ¥187,250 on
top of the sticker price, which makes the purchase the gate and the rebuild the
sink, and gives the upgrade ladder somewhere to go once the first car is maxed.

Tiers therefore live on `player_cars`, one row per owned car, and
`players.chassis_id` is a pointer to whichever one is being raced.

**Setup** — seven adjustments, and you can only turn what you have bought the
part for. A stock car has no coilovers to wind, no adjustable arms, no boost
controller; each upgrade tier hands over another spanner, which gives the
upgrade ladder a second payoff beyond raw numbers. Suspension carries three of
the seven, because that is where setup work actually lives.

Adjusting is free and unlimited — the money went on the parts. You are told
which pass you are racing before you commit, and it does not move until you
have raced it.

### The road is described, not labelled

A corner has a radius, a length, a width, a camber, a gradient and a surface,
plus whether it crests, whether it is blind, and which way it turns. Everything
else is derived from that:

- **How long it takes** comes from a quasi-steady-state speed pass — work out
  what each corner allows, walk forwards limited by acceleration, backwards
  limited by braking, then integrate accelerate-hold-brake across each piece of
  road. Lap times land between 55 and 67 seconds across the six passes without
  anyone typing a duration.
- **How dangerous it is** comes from width, camber, sight, surface and how much
  grip the corner is already using.
- **What kind of corner it is** comes from its radius and what leads into it, so
  the attribute weights hang off geometry rather than off a label.

Where published figures for a road exist, the authored geometry is calibrated
against them and the figures are recorded in the track's `RealWorld` field.
Irohazaka is documented at 6–7% average gradient rising above 9%, and the
authored version runs 7.4% with sections at 9–11%. Usui is documented at 184
corners over about 11km with roughly 550m of elevation — 17 corners per
kilometre at 4.9% — and was rebuilt to that density, which took it from 4.9
corners per kilometre to 10.6 and lifted its setup sensitivity from the worst
in the game. Mount Haruna's real road is a normal narrow two-lane mountain
road rather than the wide one the anime drew, which the 5.6–7.4m widths match;
its east face averages about 7%, and Akina's steepest corners came down from
10% to 9% to land there.

The three descents are also authored to be *different descents*, because three
roads that ask the same question are one road. Irohazaka is the long braking
problem, Akina is five hairpin exits in a row with the straights cut back so
they dominate it, and Akagi flows — linked medium-radius corners that never let
the car settle, which is what "rhythm section after rhythm section" should mean
and did not while it had a 350-metre straight into a heavy braking zone.

Those are cross-checked descriptions, not surveys. The passes remain
**authored approximations** — Irohazaka's forty-eight one-way hairpins, Usui's damp stone-walled
narrowness, Akina's five in a row — not surveys of them. Every track says so
in its own data (`source: "authored"`) and names the road it stands for, so
the claim is in the API rather than buried in a comment.

**Replacing that with measured geometry is one command:**

```bash
cd scripts
./gen_tracks.py --manifest tracks/manifest.json \
    --out ../server/internal/game/track_data.go --survey --report
```

`--survey` asks the Overpass API for each road in the manifest, stitches the
OSM ways back into one continuous line, asks an elevation service how high
every point on it is, and writes the result as a GPX next to the manifest. The
rest of the run measures those GPX files into `track_data.go`, and
`mergeSurveyed` prefers a measured track over the authored stand-in with the
same ID — so the switchover happens one pass at a time with no other change
anywhere. Without `--survey`, or with the recordings already present, nothing
touches the network.

Four things that command has to get right, none of which are obvious:

- **A road is not one way.** OSM splits it at every junction, bridge and change
  of tagging, and Overpass returns the pieces in no order, some running
  backwards. Laid end to end as they arrive they make a road that teleports,
  and a curvature pipeline reads every jump as a corner. So the pieces are
  matched by their endpoints, the longest chain is taken, and the side road the
  bounding box caught gets dropped.
- **An OSM way has no elevation**, and gradient is most of what makes a descent
  a descent. Hence the second request, batched a hundred points at a time.
- **Nothing in OSM says which end is the top.** A pass the manifest calls a
  descent is turned to run downhill once the heights arrive.
- **The real roads are 6–12km and a pass here is about 1.5km**, which is what
  race pacing, stamina and replay length are all built on. A manifest entry can
  name the stretch it wants, and the track's real-world note then says which
  stretch it is.

Length, radius, direction, gradient and where corners begin and end are
measured. Width and surface come from OSM tags when the way carries them. How
blind a corner is, whether a crest unsettles the car, and which stretch of road
to use, a GPS trace cannot know — those stay a judgement.

A bounding box is a blunt instrument, so the manifest also carries the
published figures for each road and the importer says so out loud when its
measurement lands outside them. Irohazaka descending 120 metres over 20 corners
is right; 30 metres over 200 corners means the trace is noisy or doubled back.

The importer has no real road to check itself against in CI, so it checks
itself against shapes whose answers are known exactly. A circle of radius R
has to measure R; five linked hairpins have to come out as five hairpins that
alternate; and both have to survive several metres of GPS noise. Measured:
clean geometry recovers to within 1.6%, noisy to within 8%, and five linked
16-metre hairpins come back as R15.9–16.5, LRLRL.

CI has no network either, so the fetch is checked the same way rather than
assumed: stand-ins for Overpass and the elevation service are served over real
HTTP on localhost, returning a road whose shape is known exactly, with its ways
shuffled, one of them reversed and a decoy spur mixed in. The importer is
pointed at them and asked what it made of the road — 3 ways stitched, 1
dropped, no jump larger than 3.1m where the pieces met, four R20 hairpins
recovered, and a GPX left behind that reads back pointing downhill.

Three things had to be got right for that, and all three are written down in
the code because each one looked like a different bug at first:

- **Three points are not enough.** Menger curvature uses exactly three samples
  however many are available, so noise lands straight on the answer — a 60m
  sweeper read as 17m. A least-squares circle fit over the whole window
  averages it down.
- **The window has to be chosen by evidence, not by a guess.** Estimating the
  radius and sizing the window to it is a trap: a wrong estimate picks a short
  window, and a short window confirms it. The scale is picked by fit residual
  instead, judged against the trace's own scatter — an absolute tolerance
  cannot serve both a clean export and a phone under trees.
- **The fixture lied before the code did.** An early test chained arcs by
  translating each onto the last without rotating it to match the heading,
  producing a zigzag with a kink at every join. The pipeline reported wild
  alternating curvature, which is the correct description of that road and
  looked exactly like a measurement bug.

Each is authored to disagree with itself: Happogahara is wide and fast and then
drops to a second-gear corner on broken concrete, so the low, stiff,
long-geared car that owns its straights has to give something back.

### Conditions

The pass is never the same twice. Damp and surface temperature are pinned per
race by the same pure-function trick as the track, and revealed before you
tune — so it is a decision, just never the same decision. Wet moves the answer
a long way: on Akagi the optimum ride height flips from slammed to high.

### Crashing

A sheet with no margin left in it — no ride height to absorb a bump, no travel
in the dampers, on the edge of the tyre — can put the car into the barrier. The
run ends there and the repair bill is a proportion of what has been spent on
the car.

Measured on Usui, the nastiest pass in the game: a factory car crashes 0.47% of
the time dry, and a fully wound sheet on a wet cold night 3.35%. The same wild
sheet on wide, smooth Happogahara only crashes 0.78% — geometry reaches the
crash model, so the risk is a property of the road as much as the car. A
consistent driver on the same sheet crashes 1.52% where a sloppy one crashes
4.65%.

### The rule that makes setup interesting

**A setup moves performance around. It never creates any.**

Every axis is *centred*: whatever a setting gives you on one kind of road, it
takes back elsewhere in exact proportion to how much of the game's racing
happens there. Overall rating is therefore invariant to tuning — the garage
number stays honest, and the per-track number is the one that responds.

Getting that exactly right took two corrections worth recording:

- Centring against flat time left a residue on any car whose performance is not
  flat across corner types. A rating is a *performance*-weighted average, so an
  AWD car — quicker on straights, slower through hairpins before the setup says
  anything — got a free tenth. Weighting the shares by `drivetrainMod` fixes it
  at the source.
- The same problem returns, larger, for any car far from the reference build: a
  fully built car gained about **1.7%** overall from simply winding every axis
  positive, which is a free lunch and a single obvious answer. `WithSetup`
  divides by the build's own performance-weighted average of the modifier,
  which removes it exactly for every car.

A perfect setup is worth about **5%** — roughly a ladder rung, so a well-tuned
car beats a slightly better one that is not. Measured per pass it runs 4.0% to
5.9%, averaging 4.9%.

It has to be worth about the same **everywhere**, too, or only one road is
really a tuning screen. Happogahara used to pay 7.8% against a 3.5% floor,
which is two different games depending on which pass came up; it now has a
second awkward section — it climbs out of the infield onto patched concrete and
doubles back twice — and the spread across the six is 1.5x rather than 2.2x.

| Pass | Perfect setup was worth | Now |
|---|---|---|
| Mt. Akina | 3.5% | 5.3% |
| Myogi | 5.2% | 4.0% |
| Irohazaka | 4.9% | 4.8% |
| Happogahara | **7.8%** | 5.2% |
| Mt. Akagi | 3.5% | 5.9% |
| Usui | 3.6% | 4.1% |

The optimum has to depend on the **car**, not only the road, or one published
sheet per pass would serve everybody. Gearing reads power-to-weight, boost
reads how laggy the turbo actually is, ride height and damping read weight,
camber reads existing grip — and weight balance reads which axle drives, so
moving mass forward helps a front-driven car out of a slow corner and hurts
everything else. That last one is a sign change rather than a scale, which is
what a layout needs in order to disagree.

Half the passes used to be **layout-blind**: every drivetrain wanted its mass
forward and the driven axle got no vote. That was a modelling error rather than
a fact about mountains. A downhill straight where the car is accelerating the
whole way was counted as a braking zone, because gradient was *added* to the
braking demand rather than multiplied into it — and a steep pass is mostly
that, so every descent came out braking-dominated. Three things followed from
fixing it:

- gradient now scales the braking that actually happens, plus a term for
  corners, where holding a speed downhill really is braking;
- ballast has diminishing returns where gravity has already loaded the front,
  because by then the tyre is the limit rather than the load;
- and the driven axle feels the slope. Gravity unloads the rear of a car going
  down and the front of one going up, so a rear-driven car feels a descent and
  a front-driven one feels a climb — which is exactly where the driven axle
  should have the loudest opinion.

All six passes now split, and they split differently:

| Pass | Wants mass forward | Wants it back |
|---|---|---|
| Mt. Akina | FF | FR, MR, AWD |
| Myogi | FR, MR | FF, AWD |
| Irohazaka | FR, FF, AWD | MR |
| Happogahara | FR, MR, AWD | FF |
| Mt. Akagi | FR, MR, AWD | FF |
| Usui | FF | FR, MR, AWD |

`TestSetupCannotRaiseOverallRating`, `TestNoAxisHasAUniversallyCorrectSetting`,
`TestTheBestSetupDependsOnTheCar` and `TestASetupForOnePassIsWrongOnAnother`
hold these down, and the calibration curve is unmoved at 65.2%.

### Set in millimetres, not in notches

Every axis is continuous and physical. A coilover does not have five positions;
it has a thread, so ride height is any millimetre between 130 and 70, camber
any tenth of a degree, the final drive any hundredth. `SetupAxis.Values` is the
calibration curve between the position the model works in and the number on the
sheet, and `Step` is the granularity the workshop actually works to — the
server snaps to whole steps out from the factory setting, so nothing can store
a car that could not be built, and an untouched car stays exactly untouched.

```
POST /v1/setup   {"axis": "ride_height", "setting": 87}
```

The screen offers three ways in, because there are three things you want to do
with a setting: drag the bar to find the ends of the range, tap the arrows to
walk one step, or type 87 when you already know it is 87mm you want.

### You find out by racing

Nothing predicts what a sheet is worth. The tuning screen shows the real
settings — 87mm, 2.35 bar, 4.22 — the road, and the weather, and takes no view
at all on whether any of it is right. There is no per-track score anywhere in
the API, because a number that responds to every adjustment is a dial to
optimise against, and the first player to solve it solves the pass for
everybody.

What you get instead arrives after the run, from the same terms in the model
that did the damage:

> · The floor found the road through The Gutter Corner.
> · The car skated over the broken surface in The Gutter Corner instead of settling.

It says what the car did. It never says which way to turn the spanner.

### Rivals tune too

Seeded rivals carry setups of their own, deterministic from the seed. Nobody at
the bottom of the ladder has touched the car; the top runs committed sheets,
which is also where the crash model starts to bite them.

They are set up for *something* rather than for the road they happen to be
racing on — the position a human is in when they cannot be bothered to re-tune.
So a player's edge from reading tonight's pass survives, but it is earned
rather than granted by the rivals being asleep.

### Risk is a separate axis from speed

Spread and mistakes are not the same kind of thing, and the difference is what
makes aggression a strategy rather than a tax. A wider distribution is worth
having when you are the slower car — the only way past a better build is a run
that goes unusually well. A mistake is time lost and never time found.

So aggression moves spread hard and mistakes gently. It also has to move the
*correlated* term: per-corner noise is drawn independently eleven times and
averages itself away by the finish, while form is correlated across the whole
run. Scaling segment noise alone did nothing measurable; scaling form turns a
+5% favourite's 65% into a **36.7%** underdog win rate on a wild sheet, against
35.0% on a calm one.

**Driver attributes** — bravery, intelligence, reflexes, consistency, stamina.
Three points per level, spent by the player, and they genuinely change how a
race runs rather than adding a flat bonus:

- **Bravery** — commitment through fast corners, *and* a higher mistake rate.
- **Intelligence** — better lines, fewer errors.
- **Reflexes** — hairpins and quick direction changes.
- **Consistency** — narrows your entire distribution. The stat that converts a
  favourite into a reliable favourite.
- **Stamina** — holds performance through the last third of the run.

**Ladder** — beat someone above you and take their rank; everyone between shifts
down one. Losing costs nothing but the attempt. Five free races a day, extra
runs at ◆15, capped at 20 so nobody buys their way up the mountain in an hour.

**The async opponent** is another real player's saved build, read at race time.
They are never notified and never consulted; their own progress is untouched
apart from rank. Seeded bots are ordinary rows with `is_bot = 1` and go through
exactly the same build pipeline — so when real players arrive, nothing about
matchmaking changes.

---

## Items

Rare items drop from jobs and from beating strong rivals. Drop quality scales
with the opponent you beat, so the named items enter circulation at the top of
the mountain.

| Item | Slot | Effect |
|---|---|---|
| **Keiichi's Gloves** | Hands | +6 reflexes, +3 bravery |
| **Takumi's Brakes** | Car | +10 braking, +4 consistency |
| **Ryosuke's Knowledge** | Mind | +8 intelligence, +3 consistency |
| **Bunta's Paper Cup** | Charm | +6 consistency, +4 intelligence |
| **Sandals of the God Foot** | Feet | +7 bravery, +2 reflexes |
| **God Hand Helmet** | Head | +5 intelligence, +4 reflexes |
| **Emperor Team Overalls** | Suit | +6 stamina, +4 bravery |

Every item rolls a quality multiplier (0.88–1.18) on drop, so two copies of the
same item are not the same item.

**K's Office** stocks six items a day, rotating per player. Stock is a pure
function of `(player, day)` — it is never stored, and re-rolled server-side on
every purchase to verify the price. Refreshing the app cannot reroll it, and
each offer sells once. Legendaries are priced at 4× their base value: the shop
is the impatient path, not the cheap one.

### Art

Cars are top-down 8-bit sprites, one per chassis, generated by
`scripts/gen_sprites.py` into `client/src/pixel/sprites.ts` as pixel rows. The
renderer collapses each row into horizontal runs and draws them as SVG rects —
a car is about 60 shapes rather than 680 cells.

The palette is indexed, which is the point: `1` is the livery's primary and `2`
its secondary, so all 49 chassis-and-livery combinations come out of one sprite
each. Glass, tyres and lights are fixed so a car stays readable whatever paint
is on it. To redraw a car, edit its parameters in `scripts/gen_sprites.py` and
re-run it — the script also writes a PNG contact sheet to check the result.

**Liveries are cosmetic. Always.** `TestLiveriesAreCosmetic` asserts that buying
and equipping the most expensive livery in the game leaves the computed build
byte-identical.

---

## API

All endpoints except `/health`, `/v1/content` and `/v1/auth/device` need
`Authorization: Bearer <token>`.

```
POST /v1/auth/device        { device_id }         → token + player
GET  /v1/content                                  → static game tables
GET  /v1/state                                    → everything, one call
GET  /v1/me

GET  /v1/jobs
POST /v1/jobs/start         { job_id }
POST /v1/jobs/claim
POST /v1/jobs/rush

GET  /v1/garage
POST /v1/garage/upgrade     { category }
POST /v1/driver/allocate    { stat, points }

GET  /v1/cars
POST /v1/cars/buy           { chassis_id }
POST /v1/cars/equip         { chassis_id }

GET  /v1/setup
POST /v1/setup              { axis, setting } | { axis, value }
POST /v1/setup/reset

GET  /v1/inventory
POST /v1/inventory/equip    { item_id }
POST /v1/inventory/unequip  { item_id }
POST /v1/inventory/sell     { item_id }

GET  /v1/race/opponent
POST /v1/race/start
GET  /v1/race/history
GET  /v1/race/replay/{id}
GET  /v1/ladder             ?around=me | ?from=1&limit=50

GET  /v1/shop
POST /v1/shop/buy           { offer_id }
GET  /v1/liveries
POST /v1/liveries/buy       { livery_id }
POST /v1/liveries/equip     { livery_id }
POST /v1/store/purchase     { pack_id, receipt }
```

---

## Layout

```
server/
  cmd/hairpin/           entry point
  internal/game/         pure domain: content tables, race simulation, progression
  internal/store/        persistence AND transactional game operations
  internal/api/          HTTP handlers — parsing and rendering only
  internal/auth/         HMAC session tokens
client/
  app/(tabs)/            garage, jobs, race, ladder, driver, K's office
  src/                   api client, state, theme, components
scripts/smoke.py         end-to-end loop check against a running server
```

`internal/store` holds the game operations as well as the SQL. That is
deliberate: a rule split across a service layer and a storage layer is a rule
that can be bypassed. Keeping `RunRace` and its transaction in one place means
the economy has one door.

---

## Known limitations

Milestone 1 is a playable loop, not a shippable game. These are the things that
are genuinely not finished, in the order they should be addressed:

1. **In-app purchases are not verified.** With `HAIRPIN_DEV_IAP=true` any
   receipt string mints rubies. Apple `verifyReceipt` / Google Play Developer
   API validation must be wired in before release. The replay defence (unique
   receipt column) is already there.
2. **Device auth is a bearer secret.** Whoever holds the device ID *is* the
   account. That is inherent to anonymous no-signup auth, but it means no
   account recovery and no cross-device play. Needs Sign in with Apple / Google
   account linking.
3. **SQLite with a single-writer pool.** Correct, and fine for early access, but
   it will not scale past one box. All SQL is in `internal/store`.
4. **No push notifications**, so a finished job goes unannounced.
5. **No abuse telemetry.** The rate limiter blunts brute force, but there is no
   alerting on anomalous currency gain — which is what actually catches
   exploits in production.
6. **Cars are not balanced against each other.** All seven are buyable and
   drivable, but price does not track rating monotonically — the R-Series GT
   costs ¥50,000 more than the GC-Series AWD and rates lower, because it is
   1560kg with 50 handling. That may be the right call for a trade-off car, or
   the prices may need another pass.
7. **No road has actually been recorded yet.** This is the one item on the list
   that is not waiting on code. `./gen_tracks.py --survey` fetches all six
   roads, stitches them, fills in their elevation and generates the measured
   tracks in one command, and every part of that is exercised in CI against
   stand-ins for the two services — but the environment this was built in
   refuses the real ones:

   ```
   could not reach https://overpass-api.de/api/interpreter: Tunnel connection failed: 403 Forbidden
   ```

   Run that command anywhere with network access and the passes stop being
   approximations. Two caveats when you do: the bounding boxes in
   `scripts/tracks/manifest.json` are approximate, so check each query in
   overpass-turbo before trusting what it selected (the importer cross-checks
   its measurements against the published figures and complains, which catches
   the worst of it); and a recording you made by driving the road beats
   anything a query can produce, because it carries the surface you actually
   felt.
8. **Blind corners and crests are still authored**, and always will be from a
   GPS trace — it cannot know what you can see. They are applied on top of
   measured geometry, so a surveyed road is measured in its shape and judged in
   its sightlines.

## What Milestone 2 might be

- Teams, and team battles against another team's five saved builds
- Track-specific tuning (gearing for Happogahara vs Akina)
- A real ladder season with resets and rewards
- Item set bonuses (all Project D gear equipped)
