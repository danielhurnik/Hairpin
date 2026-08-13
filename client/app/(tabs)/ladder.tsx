import React, { useCallback, useEffect, useState } from 'react';
import { Text, View } from 'react-native';
import { api } from '../../src/api';
import { Button, Card, Loading, Row, Screen, SectionLabel } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, radius, space, text as t } from '../../src/theme';
import type { LadderEntry } from '../../src/types';

export default function LadderScreen() {
  const { state, refresh } = useGame();
  const [entries, setEntries] = useState<LadderEntry[] | null>(null);
  const [mode, setMode] = useState<'me' | 'top'>('me');

  const load = useCallback(async (which: 'me' | 'top') => {
    const path = which === 'me' ? '/v1/ladder?around=me&limit=25' : '/v1/ladder?from=1&limit=25';
    const res = await api.get<{ entries: LadderEntry[] }>(path);
    setEntries(res.entries);
  }, []);

  useEffect(() => {
    void load(mode);
  }, [load, mode]);

  if (!state) return null;

  return (
    <Screen
      onRefresh={async () => {
        await Promise.all([refresh(), load(mode)]);
      }}>
      <Row gap={2}>
        <Button
          title="Around you"
          variant={mode === 'me' ? 'primary' : 'ghost'}
          style={{ flex: 1 }}
          onPress={() => setMode('me')}
        />
        <Button
          title="Top of the mountain"
          variant={mode === 'top' ? 'primary' : 'ghost'}
          style={{ flex: 1 }}
          onPress={() => setMode('top')}
        />
      </Row>

      <SectionLabel>Standings</SectionLabel>

      {!entries ? (
        <Loading />
      ) : (
        <Card style={{ padding: 0, gap: 0, overflow: 'hidden' }}>
          {entries.map((e) => (
            <LadderRow key={e.player_id} entry={e} />
          ))}
        </Card>
      )}

      <Text style={t.tiny}>
        Beat someone above you and you take their place; everyone between shifts down one. Every
        rival here is another account's saved build — you race their car, not a script.
      </Text>
    </Screen>
  );
}

function LadderRow({ entry }: { entry: LadderEntry }) {
  return (
    <View
      style={{
        flexDirection: 'row',
        alignItems: 'center',
        paddingVertical: space(3),
        paddingHorizontal: space(4),
        gap: space(3),
        backgroundColor: entry.is_you ? colors.accent + '14' : 'transparent',
        borderLeftWidth: 3,
        borderLeftColor: entry.is_you ? colors.accent : 'transparent',
        borderBottomWidth: 1,
        borderBottomColor: colors.border,
      }}>
      <View
        style={{
          width: 34,
          height: 26,
          borderRadius: radius.sm,
          backgroundColor: entry.rank <= 3 ? colors.accent : colors.raised,
          alignItems: 'center',
          justifyContent: 'center',
        }}>
        <Text
          style={{
            fontWeight: '800',
            fontSize: 12,
            color: entry.rank <= 3 ? colors.bg : colors.textDim,
            fontVariant: ['tabular-nums'],
          }}>
          {entry.rank}
        </Text>
      </View>

      <View style={{ flex: 1 }}>
        <Text
          style={[
            t.body,
            { fontWeight: entry.is_you ? '800' : '600', color: entry.is_you ? colors.accent : colors.text },
          ]}
          numberOfLines={1}>
          {entry.name}
          {entry.is_you ? ' (you)' : ''}
        </Text>
        <Text style={t.tiny} numberOfLines={1}>
          Lv {entry.driver_level} · {entry.chassis_name} · {entry.wins}W {entry.losses}L
        </Text>
      </View>

      <Text
        style={{
          fontSize: 15,
          fontWeight: '800',
          color: colors.text,
          fontVariant: ['tabular-nums'],
        }}>
        {entry.rating.toFixed(0)}
      </Text>
    </View>
  );
}
