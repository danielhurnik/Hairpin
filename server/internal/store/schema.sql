-- Hairpin schema. Everything the client could lie about lives here and is
-- only ever written by the server.

CREATE TABLE IF NOT EXISTS players (
    id             TEXT PRIMARY KEY,
    device_hash    TEXT UNIQUE,               -- NULL for ladder bots
    display_name   TEXT NOT NULL,
    is_bot         INTEGER NOT NULL DEFAULT 0,

    cash           INTEGER NOT NULL DEFAULT 0,
    rubies         INTEGER NOT NULL DEFAULT 0,

    driver_level   INTEGER NOT NULL DEFAULT 1,
    driver_xp      INTEGER NOT NULL DEFAULT 0,
    unspent_points INTEGER NOT NULL DEFAULT 0,

    stat_bravery      REAL NOT NULL DEFAULT 10,
    stat_intelligence REAL NOT NULL DEFAULT 10,
    stat_reflexes     REAL NOT NULL DEFAULT 10,
    stat_consistency  REAL NOT NULL DEFAULT 10,
    stat_stamina      REAL NOT NULL DEFAULT 10,

    -- The car currently being raced. Upgrade tiers live on player_cars, one
    -- row per owned car, so switching cars switches its whole build with it.
    chassis_id     TEXT NOT NULL DEFAULT 'ae_coupe',
    livery_id      TEXT NOT NULL DEFAULT 'panda',

    ladder_rank    INTEGER NOT NULL,
    wins           INTEGER NOT NULL DEFAULT 0,
    losses         INTEGER NOT NULL DEFAULT 0,

    free_races       INTEGER NOT NULL DEFAULT 5,
    extra_races_used INTEGER NOT NULL DEFAULT 0,
    races_reset_at   INTEGER NOT NULL DEFAULT 0,

    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);

-- Not unique: rank shuffles temporarily collide inside a transaction.
CREATE INDEX IF NOT EXISTS idx_players_rank ON players(ladder_rank);
CREATE INDEX IF NOT EXISTS idx_players_bot ON players(is_bot);

CREATE TABLE IF NOT EXISTS player_jobs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id    TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    job_id       TEXT NOT NULL,
    started_at   INTEGER NOT NULL,
    completes_at INTEGER NOT NULL,
    claimed_at   INTEGER,
    rushed       INTEGER NOT NULL DEFAULT 0
);

-- At most one unclaimed job per player, enforced by the database rather than
-- by hoping two concurrent requests do not interleave.
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_job
    ON player_jobs(player_id) WHERE claimed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_jobs_player ON player_jobs(player_id, id DESC);

CREATE TABLE IF NOT EXISTS inventory (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id   TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    item_id     TEXT NOT NULL,
    slot        TEXT NOT NULL,
    quality     REAL NOT NULL DEFAULT 1,
    equipped    INTEGER NOT NULL DEFAULT 0,
    source      TEXT NOT NULL DEFAULT 'drop',
    acquired_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inventory_player ON inventory(player_id);
-- One equipped item per slot, enforced in the schema.
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_per_slot
    ON inventory(player_id, slot) WHERE equipped = 1;

-- Every car a player owns, with that car's own upgrade tiers. A player's
-- active car is players.chassis_id, which always has a row here.
--
-- Tiers are per-car on purpose: buying a chassis gives you a stock chassis,
-- not a rebadged version of the car you already built. The primary key is
-- what stops the same car being bought twice.
CREATE TABLE IF NOT EXISTS player_cars (
    player_id   TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    chassis_id  TEXT NOT NULL,
    tier_engine     INTEGER NOT NULL DEFAULT 0,
    tier_turbo      INTEGER NOT NULL DEFAULT 0,
    tier_tires      INTEGER NOT NULL DEFAULT 0,
    tier_suspension INTEGER NOT NULL DEFAULT 0,
    tier_weight     INTEGER NOT NULL DEFAULT 0,
    -- The setup sheet, one per car. Zero is however it left the workshop; an
    -- axis is only adjustable once the tier above has fitted the part for it,
    -- and anything set on a part that is not fitted is stripped on read.
    setup_gearing   INTEGER NOT NULL DEFAULT 0,
    setup_boost     INTEGER NOT NULL DEFAULT 0,
    setup_pressure  INTEGER NOT NULL DEFAULT 0,
    setup_ballast   INTEGER NOT NULL DEFAULT 0,
    setup_ride      INTEGER NOT NULL DEFAULT 0,
    setup_camber    INTEGER NOT NULL DEFAULT 0,
    setup_damping   INTEGER NOT NULL DEFAULT 0,
    acquired_at INTEGER NOT NULL,
    PRIMARY KEY (player_id, chassis_id)
);

CREATE TABLE IF NOT EXISTS player_liveries (
    player_id   TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    livery_id   TEXT NOT NULL,
    acquired_at INTEGER NOT NULL,
    PRIMARY KEY (player_id, livery_id)
);

CREATE TABLE IF NOT EXISTS races (
    id             TEXT PRIMARY KEY,
    challenger_id  TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    opponent_id    TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    opponent_name  TEXT NOT NULL,
    track_id       TEXT NOT NULL,
    seed           INTEGER NOT NULL,
    challenger_won INTEGER NOT NULL,
    margin         REAL NOT NULL,
    cash_awarded   INTEGER NOT NULL,
    xp_awarded     INTEGER NOT NULL,
    rank_before    INTEGER NOT NULL,
    rank_after     INTEGER NOT NULL,
    drop_item_id   TEXT,
    replay         TEXT NOT NULL,
    created_at     INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_races_player ON races(challenger_id, created_at DESC);

-- One purchase per rotating shop offer.
CREATE TABLE IF NOT EXISTS shop_purchases (
    player_id   TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    offer_id    TEXT NOT NULL,
    day_key     TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    PRIMARY KEY (player_id, offer_id)
);

-- Receipts are globally unique so the same purchase can never be redeemed twice.
CREATE TABLE IF NOT EXISTS ruby_purchases (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id  TEXT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    pack_id    TEXT NOT NULL,
    receipt    TEXT NOT NULL UNIQUE,
    rubies     INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
