import { Link } from 'expo-router';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { ApiError, api } from '../../src/api';
import { CarArt, DriverStatBlock, LiveryColors, RarityBadge, st } from '../../src/components/game';
import { RaceHistory } from '../../src/components/history';
import { GapMeter, SectorTable } from '../../src/components/replay';
import { Banner, Button, Card, Loading, Row, Screen, SectionLabel } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, rarityColor, space, text as t } from '../../src/theme';
import type { Opponent, RaceOutcome } from '../../src/types';

type OpponentResponse = {
  opponent: Opponent;
  your_rating: number;
  free_races: number;
  extra_cost: number;
};

/** How long each corner takes to reveal during the replay. */
const SEGMENT_MS = 550;

export default function RaceScreen() {
  const { state, refresh } = useGame();
  const [preview, setPreview] = useState<OpponentResponse | null>(null);
  const [outcome, setOutcome] = useState<RaceOutcome | null>(null);
  const [revealed, setRevealed] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadPreview = useCallback(async () => {
    setError(null);
    try {
      setPreview(await api.get<OpponentResponse>('/v1/race/opponent'));
    } catch (e) {
      setPreview(null);
      setError(e instanceof ApiError ? e.message : 'Could not find an opponent');
    }
  }, []);

  useEffect(() => {
    if (!outcome) void loadPreview();
  }, [loadPreview, outcome]);

  useEffect(() => () => void (timer.current && clearInterval(timer.current)), []);

  const start = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await api.post<RaceOutcome>('/v1/race/start');
      setOutcome(res);
      setRevealed(0);
      // Reveal the run corner by corner. The result is already decided — this
      // is presentation of a record the server wrote, not a live simulation.
      timer.current = setInterval(() => {
        setRevealed((n) => {
          const next = n + 1;
          if (next >= res.replay.segments.length && timer.current) {
            clearInterval(timer.current);
            timer.current = null;
            void refresh();
          }
          return next;
        });
      }, SEGMENT_MS);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Race failed');
    } finally {
      setBusy(false);
    }
  };

  const skip = () => {
    if (timer.current) {
      clearInterval(timer.current);
      timer.current = null;
    }
    if (outcome) setRevealed(outcome.replay.segments.length);
    void refresh();
  };

  const reset = () => {
    setOutcome(null);
    setRevealed(0);
  };

  if (!state) return null;

  if (outcome) {
    return (
      <ReplayView
        outcome={outcome}
        revealed={revealed}
        onSkip={skip}
        onAgain={reset}
        racesLeft={outcome.free_races_left}
      />
    );
  }

  const canRace = state.player.free_races > 0 || state.player.rubies >= 15;

  return (
    <Screen onRefresh={refresh}>
      {error && <Banner tone="error">{error}</Banner>}

      <Card>
        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={t.label}>Races today</Text>
          <Text style={t.body}>
            {state.player.free_races} free
            {state.player.free_races === 0 ? ' · ◆15 per extra run' : ''}
          </Text>
        </Row>
      </Card>

      <TonightCard />

      {!preview ? (
        <Loading label="Finding someone on the pass…" />
      ) : (
        <>
          <SectionLabel>Next challenge</SectionLabel>
          <OpponentCard opponent={preview.opponent} yourRating={preview.your_rating} />

          <Card>
            <Text style={t.label}>Head to head</Text>
            <Row gap={4} style={{ alignItems: 'flex-start' }}>
              <View style={{ flex: 1, gap: space(2) }}>
                <Text style={[t.small, { color: colors.accent }]}>You</Text>
                <DriverStatBlock stats={state.build.driver} compare={preview.opponent.build.driver} />
              </View>
            </Row>
          </Card>

          <Button
            title={state.player.free_races > 0 ? 'Run it' : 'Run it — ◆15'}
            variant="primary"
            busy={busy}
            disabled={!canRace}
            subtitle={canRace ? undefined : 'Out of races and rubies — come back tomorrow'}
            onPress={start}
          />
          <Button title="Different opponent" variant="ghost" onPress={loadPreview} />
        </>
      )}

      <RaceHistory />
    </Screen>
  );
}

/**
 * The road and the weather, before you commit to them. The full sheet lives on
 * the Setup tab; this is the reminder that there is one, and that it is set for
 * a specific road on a specific night.
 */
function TonightCard() {
  const { state } = useGame();
  if (!state) return null;
  const { track, conditions, axes, any_unlocked } = state.setup;
  const moved = axes.filter((a) => a.unlocked && Math.abs(a.setting - a.factory) > a.step / 2);

  return (
    <Card>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1 }}>
          <Text style={t.label}>Tonight's pass</Text>
          <Text style={t.heading}>{track.name}</Text>
          <Text style={t.small}>{conditions.label}</Text>
        </View>
        <Link href="/setup" asChild>
          <Pressable>
            <Text style={[t.small, { color: colors.accent }]}>Set the car up →</Text>
          </Pressable>
        </Link>
      </Row>
      <Text style={t.tiny}>
        {!any_unlocked
          ? 'Nothing on this car is adjustable yet — upgrades unlock the spanners.'
          : moved.length === 0
            ? 'Running the settings it left the workshop on.'
            : moved
                .slice(0, 4)
                .map((a) => `${a.label} ${a.display}`)
                .join(' · ') + (moved.length > 4 ? ` · +${moved.length - 4} more` : '')}
      </Text>
    </Card>
  );
}

function OpponentCard({ opponent, yourRating }: { opponent: Opponent; yourRating: number }) {
  const { state } = useGame();
  const livery = state?.liveries.find((l) => l.id === opponent.livery_id);
  const { primary, secondary } = LiveryColors(livery);
  const edge = yourRating - opponent.rating;

  return (
    <Card>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1 }}>
          <Text style={t.label}>Rank {opponent.ladder_rank}</Text>
          <Text style={t.title}>{opponent.name}</Text>
          <Text style={t.small}>
            Level {opponent.driver_level} · {opponent.chassis_name}
          </Text>
        </View>
        <View style={{ alignItems: 'flex-end' }}>
          <Text style={t.label}>Rating</Text>
          <Text style={[st.ratingValue, { color: colors.text }]}>{opponent.rating.toFixed(1)}</Text>
          <Text style={[t.tiny, { color: edge >= 0 ? colors.green : colors.red }]}>
            you {edge >= 0 ? '+' : ''}
            {edge.toFixed(1)}
          </Text>
        </View>
      </Row>

      <View style={{ alignItems: 'center' }}>
        <CarArt primary={primary} secondary={secondary} width={92} chassisId={opponent.chassis_id} />
      </View>

      {opponent.notable_items?.length ? (
        <View style={{ gap: space(1) }}>
          <Text style={t.label}>Carrying</Text>
          {opponent.notable_items.map((n) => (
            <Text key={n} style={[t.small, { color: colors.purple }]}>
              {n}
            </Text>
          ))}
        </View>
      ) : null}

      <Banner tone="info">
        {edge >= 0
          ? 'You are the favourite — which is worth about two runs in three.'
          : 'You are the underdog. The mountain gets a vote.'}
      </Banner>
    </Card>
  );
}

function ReplayView({
  outcome,
  revealed,
  onSkip,
  onAgain,
  racesLeft,
}: {
  outcome: RaceOutcome;
  revealed: number;
  onSkip: () => void;
  onAgain: () => void;
  racesLeft: number;
}) {
  const segs = outcome.replay.segments;
  const done = revealed >= segs.length;
  const shown = segs.slice(0, revealed);
  const current = shown[shown.length - 1];

  return (
    <Screen>
      <Card>
        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={t.label}>{outcome.replay.track_name}</Text>
          <Text style={t.label}>{outcome.replay.conditions?.label}</Text>
        </Row>
        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={t.heading}>You</Text>
          <Text style={t.heading}>{outcome.opponent.name}</Text>
        </Row>

        <GapMeter gap={current?.gap ?? 0} />

        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={st.timeCell}>{(current?.challenger_total ?? 0).toFixed(2)}</Text>
          <Text style={[t.small, { flex: 1, textAlign: 'center' }]}>
            {current ? current.name : 'On the line'}
          </Text>
          <Text style={st.timeCell}>{(current?.opponent_total ?? 0).toFixed(2)}</Text>
        </Row>

        {current?.note ? (
          <Text style={[t.small, { textAlign: 'center', color: colors.accent }]}>{current.note}</Text>
        ) : null}
      </Card>

      <Card>
        <SectionLabel>Sector times</SectionLabel>
        <SectorTable segments={shown} />
        {!done && <Loading />}
      </Card>

      {done && (
        <>
          {outcome.crashed && (
            <Card>
              <Text style={[t.title, { color: colors.red }]}>Into the barrier</Text>
              <Text style={t.small}>
                {outcome.replay.crash_segment
                  ? `It let go at ${outcome.replay.crash_segment}. `
                  : ''}
                The run ends there — no reward, and the car has to be put straight.
              </Text>
              <Text style={[t.body, { color: colors.red }]}>
                −¥{(outcome.repair_cost ?? 0).toLocaleString()} in repairs
              </Text>
              <Text style={t.tiny}>
                A sheet with nothing left in reserve is quick right up until it is not. Ride
                height, damping, camber and boost are the settings that take the margin away.
              </Text>
            </Card>
          )}

          {outcome.setup_notes?.length ? (
            <Card>
              <Text style={t.label}>What the car did</Text>
              {outcome.setup_notes.map((note) => (
                <Row key={note.segment + note.text} gap={2} style={{ alignItems: 'flex-start' }}>
                  <Text style={[t.small, { color: colors.accent }]}>•</Text>
                  <Text style={[t.small, { flex: 1 }]}>{note.text}</Text>
                </Row>
              ))}
              <Text style={t.tiny}>
                Where the sheet cost you time, and nothing about which way to turn the spanner.
                That part is yours.
              </Text>
            </Card>
          ) : null}

          <Card tone="accent">
            <Text style={[t.title, { color: outcome.won ? colors.green : colors.red }]}>
              {outcome.won ? 'WIN' : 'LOSS'} by {outcome.replay.margin.toFixed(2)}s
            </Text>
            <Text style={t.small}>
              {outcome.replay.lead_changes > 0
                ? `${outcome.replay.lead_changes} lead change${outcome.replay.lead_changes > 1 ? 's' : ''}`
                : 'Led from lights to flag'}
              {outcome.replay.upset ? ' · upset' : ''}
            </Text>

            <Row gap={4}>
              <Text style={[t.body, { color: colors.cash }]}>
                +¥{outcome.reward.cash.toLocaleString()}
              </Text>
              <Text style={[t.body, { color: colors.blue }]}>+{outcome.reward.xp} XP</Text>
            </Row>

            {outcome.rank_after < outcome.rank_before ? (
              <Banner tone="good">
                Ladder {outcome.rank_before} → {outcome.rank_after}
              </Banner>
            ) : (
              <Text style={t.small}>Ladder unchanged at {outcome.rank_after}</Text>
            )}

            {outcome.leveled_to ? (
              <Banner tone="good">
                Level {outcome.leveled_to} — {outcome.stat_points_awarded} points to spend.
              </Banner>
            ) : null}

            {outcome.drop ? (
              <View style={{ gap: space(1) }}>
                <Text style={t.label}>Dropped</Text>
                <Row gap={2}>
                  <Text style={[t.body, { color: rarityColor[outcome.drop.rarity] }]}>
                    {outcome.drop.name}
                  </Text>
                  <RarityBadge rarity={outcome.drop.rarity} />
                </Row>
                <Text style={[t.tiny, { fontStyle: 'italic' }]}>{outcome.drop.flavour}</Text>
              </View>
            ) : null}
          </Card>

          <Button
            title={racesLeft > 0 ? `Race again (${racesLeft} free left)` : 'Race again — ◆15'}
            variant="primary"
            onPress={onAgain}
          />
        </>
      )}

      {!done && <Button title="Skip to result" variant="ghost" onPress={onSkip} />}
    </Screen>
  );
}
