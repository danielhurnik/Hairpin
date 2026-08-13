// Mirrors the server's JSON. The client renders these; it never derives them.

export type DriverStats = {
  bravery: number;
  intelligence: number;
  reflexes: number;
  consistency: number;
  stamina: number;
};

export type CarStats = {
  power: number;
  weight: number;
  power_to_wt: number;
  grip: number;
  handling: number;
  braking: number;
  drivetrain: string;
  turbo_lag: number;
};

export type Build = { car: CarStats; driver: DriverStats };

export type Player = {
  id: string;
  display_name: string;
  is_bot: boolean;
  cash: number;
  rubies: number;
  driver_level: number;
  driver_xp: number;
  driver_xp_to_next: number;
  unspent_points: number;
  base_stats: DriverStats;
  chassis_id: string;
  upgrades: {
    engine: number;
    turbo: number;
    tires: number;
    suspension: number;
    weight: number;
  };
  livery_id: string;
  ladder_rank: number;
  wins: number;
  losses: number;
  free_races: number;
  extra_races_used: number;
  races_reset_at: string;
  created_at: string;
};

export type JobOffer = {
  id: string;
  name: string;
  flavour: string;
  tier: number;
  duration_seconds: number;
  cash: number;
  xp: number;
  min_level: number;
  drop_chance: number;
  locked: boolean;
  unlocks_at_level: number;
};

export type ActiveJob = {
  id: number;
  job_id: string;
  name: string;
  tier: number;
  started_at: string;
  completes_at: string;
  remaining_seconds: number;
  ready: boolean;
  rush_rubies: number;
  cash: number;
  xp: number;
};

export type UpgradeOption = {
  category: string;
  label: string;
  tier: number;
  max_tier: number;
  tier_name: string;
  next_tier_name?: string;
  next_cost?: number;
  affordable: boolean;
  maxed: boolean;
  rating_delta?: number;
};

export type InventoryItem = {
  id: number;
  item_id: string;
  name: string;
  slot: string;
  rarity: string;
  flavour: string;
  quality: number;
  equipped: boolean;
  source: string;
  sell_value: number;
  acquired_at: string;
};

export type LadderEntry = {
  rank: number;
  player_id: string;
  name: string;
  driver_level: number;
  is_bot: boolean;
  chassis_name: string;
  livery_id: string;
  rating: number;
  wins: number;
  losses: number;
  is_you: boolean;
};

export type LiveryOption = {
  id: string;
  name: string;
  primary: string;
  secondary: string;
  price_cash: number;
  price_rubies: number;
  starter: boolean;
  owned: boolean;
  equipped: boolean;
};

export type Opponent = {
  player_id: string;
  name: string;
  ladder_rank: number;
  driver_level: number;
  is_bot: boolean;
  chassis_id: string;
  chassis_name: string;
  livery_id: string;
  rating: number;
  build: Build;
  notable_items?: string[];
};

export type SegmentResult = {
  index: number;
  name: string;
  kind: string;
  challenger_time: number;
  opponent_time: number;
  challenger_total: number;
  opponent_total: number;
  gap: number;
  challenger_mistake: boolean;
  opponent_mistake: boolean;
  note?: string;
};

export type RaceReplay = {
  track_id: string;
  track_name: string;
  seed: number;
  segments: SegmentResult[];
  challenger_total: number;
  opponent_total: number;
  challenger_won: boolean;
  margin: number;
  challenger_rating: number;
  opponent_rating: number;
  lead_changes: number;
  upset: boolean;
};

export type RaceOutcome = {
  race_id: string;
  opponent: Opponent;
  replay: RaceReplay;
  won: boolean;
  reward: { cash: number; xp: number };
  rank_before: number;
  rank_after: number;
  drop?: InventoryItem;
  leveled_to?: number;
  stat_points_awarded?: number;
  rubies_spent?: number;
  free_races_left: number;
  player: Player;
  your_rating: number;
};

export type ShopOffer = {
  offer_id: string;
  item_id: string;
  name: string;
  slot: string;
  rarity: string;
  flavour: string;
  quality: number;
  price_cash?: number;
  price_rubies?: number;
  sold_out: boolean;
};

export type RubyPack = {
  id: string;
  name: string;
  rubies: number;
  bonus: number;
  price_usd: string;
};

export type ShopView = {
  day_key: string;
  offers: ShopOffer[];
  ruby_packs: RubyPack[];
  cash: number;
  rubies: number;
};

export type ClaimResult = {
  job: string;
  cash: number;
  xp: number;
  leveled_to?: number;
  stat_points_awarded?: number;
  drop?: InventoryItem;
  total_cash: number;
  driver_level: number;
  driver_xp: number;
  driver_xp_to_next: number;
};

export type GameState = {
  player: Player;
  build: Build;
  rating: number;
  equipped: InventoryItem[];
  inventory: InventoryItem[];
  active_job: ActiveJob | null;
  job_board: JobOffer[];
  garage: UpgradeOption[];
  ladder: LadderEntry[];
  liveries: LiveryOption[];
  server_time: string;
};
