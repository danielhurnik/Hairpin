import React, { useCallback, useEffect, useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { api } from '../api';
import { colors, space, text as t } from '../theme';
import type { RaceReplay, RaceSummary } from '../types';
import { Card, Empty, Loading, Row, SectionLabel } from './ui';
import { SectorTable } from './replay';

/**
 * Past races, read back from the server. Tapping one fetches the stored
 * corner-by-corner record — the server kept the seed, so the replay it returns
 * is the same race, re-derived rather than remembered.
 */
export function RaceHistory() {
  const [races, setRaces] = useState<RaceSummary[] | null>(null);
  const [openID, setOpenID] = useState<string | null>(null);
  const [replay, setReplay] = useState<RaceReplay | null>(null);
  const [loadingReplay, setLoadingReplay] = useState(false);

  const load = useCallback(async () => {
    const res = await api.get<{ races: RaceSummary[] }>('/v1/race/history?limit=15');
    setRaces(res.races ?? []);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const toggle = async (id: string) => {
    if (openID === id) {
      setOpenID(null);
      setReplay(null);
      return;
    }
    setOpenID(id);
    setReplay(null);
    setLoadingReplay(true);
    try {
      setReplay(await api.get<RaceReplay>(`/v1/race/replay/${id}`));
    } finally {
      setLoadingReplay(false);
    }
  };

  if (!races) return <Loading />;
  if (races.length === 0) {
    return <Empty>No runs yet. Your first race will show up here.</Empty>;
  }

  return (
    <>
      <SectionLabel>Recent runs</SectionLabel>
      {races.map((race) => (
        <Card key={race.race_id} style={{ gap: space(2) }}>
          <Pressable onPress={() => toggle(race.race_id)}>
            <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
              <View style={{ flex: 1, paddingRight: space(2) }}>
                <Row gap={2}>
                  <Text
                    style={{
                      fontWeight: '800',
                      fontSize: 12,
                      color: race.won ? colors.green : colors.red,
                    }}>
                    {race.won ? 'WIN' : 'LOSS'}
                  </Text>
                  <Text style={t.small}>{race.margin.toFixed(2)}s</Text>
                </Row>
                <Text style={[t.body, { marginTop: 2 }]} numberOfLines={1}>
                  {race.opponent}
                </Text>
                <Text style={t.tiny}>{race.track_name}</Text>
              </View>
              <View style={{ alignItems: 'flex-end' }}>
                <Text style={[t.small, { color: colors.cash }]}>
                  +¥{race.cash.toLocaleString()}
                </Text>
                {race.rank_after < race.rank_before ? (
                  <Text style={[t.tiny, { color: colors.green }]}>
                    {race.rank_before} → {race.rank_after}
                  </Text>
                ) : (
                  <Text style={t.tiny}>rank {race.rank_after}</Text>
                )}
                {race.drop_item ? (
                  <Text style={[t.tiny, { color: colors.purple }]} numberOfLines={1}>
                    {race.drop_item}
                  </Text>
                ) : null}
              </View>
            </Row>
            <Text style={[t.tiny, { marginTop: space(1), color: colors.accent }]}>
              {openID === race.race_id ? 'Hide sectors' : 'Show sectors'}
            </Text>
          </Pressable>

          {openID === race.race_id &&
            (loadingReplay ? <Loading /> : replay ? <SectorTable segments={replay.segments} /> : null)}
        </Card>
      ))}
    </>
  );
}
