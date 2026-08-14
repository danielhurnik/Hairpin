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

The passes are currently **authored approximations** of the real roads'
character — Irohazaka's forty-eight one-way hairpins, Usui's damp stone-walled
narrowness, Akina's five in a row — not surveys of them. Every track says so
in its own data (`source: "authored"`) and names the road it stands for, so
the claim is in the API rather than buried in a comment.

**Replacing that with measured geometry takes one command.** `scripts/gen_tracks.py`
turns a recorded line down the real road — a GPX from a phone, or the OSM way
via Overpass — into the same structure, measured:

```bash
cd scripts
./gen_tracks.py --self-test
./gen_tracks.py --manifest tracks/manifest.json \
    --out ../server/internal/game/track_data.go --report
```

`mergeSurveyed` prefers a measured track over the authored stand-in with the
same ID and leaves every other road alone, so the switchover happens one pass
at a time with no other change anywhere. See `scripts/tracks/README.md`.

Length, radius, direction, gradient and where corners begin and end are
measured. Width and surface come from OSM tags when the way carries them.
How blind a corner is, and whether a crest unsettles the car, a GPS trace
cannot know — those stay a judgement.

The importer has no real road to check itself against in CI, so it checks
itself against shapes whose answers are known exactly. A circle of radius R
has to measure R; five linked hairpins have to come out as five hairpins that
alternate; and both have to survive several metres of GPS noise. Measured:
clean geometry recovers to within 1.6%, noisy to within 8%, and five linked
16-metre hairpins come back as R15.9–16.5, LRLRL.

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

Measured on Usui, the nastiest pass in the game: a factory car crashes 0.33% of
the time dry, and a fully wound sheet on a wet cold night 2.27%. The same wild
sheet on wide, smooth Happogahara only crashes 0.35% — geometry reaches the
crash model, so the risk is a property of the road as much as the car. A
consistent driver on the same sheet crashes 0.88% where a sloppy one crashes
3.02%.

### The rule that makes setup interesting

**A setup moves performance around. It never creates any.**

Every axis is *centred*: whatever a notch gives you on one kind of road, it
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
car beats a slightly better one that is not. Measured per pass it runs 3.1% to
7.0%, averaging 4.6%.

The optimum has to depend on the **car**, not only the road, or one published
sheet per pass would serve everybody. Gearing reads power-to-weight, boost
reads how laggy the turbo actually is, ride height and damping read weight,
camber reads existing grip — and weight balance reads which axle drives, so
moving mass forward helps a front-driven car out of a slow corner and hurts
everything else. That last one is a sign change rather than a scale, which is
what a layout needs in order to disagree.

`TestSetupCannotRaiseOverallRating`, `TestNoAxisHasAUniversallyCorrectSetting`,
`TestTheBestSetupDependsOnTheCar` and `TestASetupForOnePassIsWrongOnAnother`
hold these down, and the calibration curve is unmoved at 65.2%.

### You find out by racing

Nothing predicts what a sheet is worth. The tuning screen shows the real
settings — 85mm, 2.4 bar, 4.30 — the road, and the weather, and takes no view
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
POST /v1/setup              { axis, value }
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
8. **Two passes barely respond to setup.** Happogahara moves five of the seven
   axes; Usui moves one. That is defensible variety, but Usui in particular is
   close to a screen with nothing to decide on it.
9. **There is no setup UI.** The sheet, the gates, the geometry, the weather
   and the post-race notes are all server-side and covered by tests; nothing
   renders any of it yet.
10. **No road has actually been recorded yet.** The importer, its self-test
    and the switchover are all in place and CI-checked, but this environment
    has no network access to OSM or elevation services, so every pass is still
    an authored stand-in. One GPX per road turns each into a measurement.
11. **Setup input is still notched, not continuous.** Every axis is physical
    internally and displays a real value, but the player picks one of five
    positions rather than typing 87mm. Exposing continuous input is now a
    small change rather than a rewrite.
12. **Happogahara pays more for a setup than the others** — 7.0% against a
    3.1% floor on Usui. Authoring a second awkward section into it would even
    that out.

## What Milestone 2 might be

- Teams, and team battles against another team's five saved builds
- Track-specific tuning (gearing for Happogahara vs Akina)
- A real ladder season with resets and rewards
- Item set bonuses (all Project D gear equipped)
