import React, { useEffect, useState } from 'react';
import { Text, View } from 'react-native';
import { ApiError } from '../../src/api';
import { RarityBadge, formatDuration } from '../../src/components/game';
import { Bar, Banner, Button, Card, Row, Screen, SectionLabel } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, rarityColor, space, text as t } from '../../src/theme';
import type { ClaimResult, JobOffer } from '../../src/types';

export default function JobsScreen() {
  const { state, act, refresh } = useGame();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [claimed, setClaimed] = useState<ClaimResult | null>(null);

  // The server owns the deadline; this only re-renders the countdown locally.
  const [, tick] = useState(0);
  const active = state?.active_job ?? null;
  useEffect(() => {
    if (!active || active.ready) return;
    const id = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, [active]);

  if (!state) return null;

  const remaining = active ? Math.max(0, (Date.parse(active.completes_at) - Date.now()) / 1000) : 0;
  const total = active
    ? Math.max(1, (Date.parse(active.completes_at) - Date.parse(active.started_at)) / 1000)
    : 1;
  const ready = active ? remaining <= 0 : false;

  const run = async (fn: () => Promise<void>, key: string) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Something went wrong');
    } finally {
      setBusy(null);
    }
  };

  return (
    <Screen onRefresh={refresh}>
      {error && <Banner tone="error">{error}</Banner>}

      {claimed && (
        <Card tone="accent">
          <Text style={t.heading}>{claimed.job} — paid out</Text>
          <Row gap={4}>
            <Text style={[t.body, { color: colors.cash }]}>+¥{claimed.cash.toLocaleString()}</Text>
            <Text style={[t.body, { color: colors.blue }]}>+{claimed.xp} XP</Text>
          </Row>
          {claimed.leveled_to ? (
            <Banner tone="good">
              Level {claimed.leveled_to} — {claimed.stat_points_awarded} stat points to spend on the
              Driver tab.
            </Banner>
          ) : null}
          {claimed.drop ? (
            <View style={{ gap: space(1) }}>
              <Text style={t.label}>Found on the job</Text>
              <Row gap={2}>
                <Text style={[t.body, { color: rarityColor[claimed.drop.rarity] }]}>
                  {claimed.drop.name}
                </Text>
                <RarityBadge rarity={claimed.drop.rarity} />
              </Row>
            </View>
          ) : null}
          <Button title="Dismiss" variant="ghost" onPress={() => setClaimed(null)} />
        </Card>
      )}

      {active ? (
        <Card tone="accent">
          <Text style={t.label}>On the clock</Text>
          <Text style={t.title}>{active.name}</Text>
          <Bar value={total - remaining} max={total} color={ready ? colors.green : colors.accent} height={8} />
          <Row style={{ justifyContent: 'space-between' }}>
            <Text style={t.small}>
              {ready ? 'Finished' : `${formatDuration(remaining)} remaining`}
            </Text>
            <Text style={t.small}>
              ¥{active.cash.toLocaleString()} · {active.xp} XP
            </Text>
          </Row>

          {ready ? (
            <Button
              title="Collect payment"
              variant="primary"
              busy={busy === 'claim'}
              onPress={() =>
                run(async () => {
                  const res = await act<ClaimResult>('/v1/jobs/claim');
                  setClaimed(res);
                }, 'claim')
              }
            />
          ) : (
            <Button
              title={`Skip the wait — ◆${active.rush_rubies}`}
              busy={busy === 'rush'}
              disabled={state.player.rubies < active.rush_rubies}
              subtitle={
                state.player.rubies < active.rush_rubies ? 'Not enough rubies' : undefined
              }
              onPress={() => run(async () => void (await act('/v1/jobs/rush')), 'rush')}
            />
          )}
        </Card>
      ) : (
        <Banner tone="info">
          Pick a job. Longer runs pay more — the timer runs on the server, so you can close the app.
        </Banner>
      )}

      <SectionLabel>Job board</SectionLabel>
      {state.job_board.map((job) => (
        <JobCard
          key={job.id}
          job={job}
          disabled={!!active}
          busy={busy === job.id}
          onStart={() => run(async () => void (await act('/v1/jobs/start', { job_id: job.id })), job.id)}
        />
      ))}
    </Screen>
  );
}

function JobCard({
  job,
  disabled,
  busy,
  onStart,
}: {
  job: JobOffer;
  disabled: boolean;
  busy: boolean;
  onStart: () => void;
}) {
  return (
    <Card style={job.locked ? { opacity: 0.55 } : undefined}>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1, paddingRight: space(3) }}>
          <Text style={t.label}>Tier {job.tier}</Text>
          <Text style={t.heading}>{job.name}</Text>
          <Text style={[t.small, { fontStyle: 'italic', marginTop: 2 }]}>{job.flavour}</Text>
        </View>
        <View style={{ alignItems: 'flex-end' }}>
          <Text style={[t.body, { color: colors.cash, fontWeight: '700' }]}>
            ¥{job.cash.toLocaleString()}
          </Text>
          <Text style={t.tiny}>{job.xp} XP</Text>
        </View>
      </Row>

      <Row style={{ justifyContent: 'space-between' }}>
        <Text style={t.small}>Takes {formatDuration(job.duration_seconds)}</Text>
        <Text style={t.tiny}>{Math.round(job.drop_chance * 100)}% chance of an item</Text>
      </Row>

      {job.locked ? (
        <Banner tone="info">Unlocks at driver level {job.unlocks_at_level}</Banner>
      ) : (
        <Button
          title="Take the job"
          variant={disabled ? 'default' : 'primary'}
          disabled={disabled}
          busy={busy}
          subtitle={disabled ? 'Finish your current job first' : undefined}
          onPress={onStart}
        />
      )}
    </Card>
  );
}
