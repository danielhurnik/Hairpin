import React, { useState } from 'react';
import { Text, TextInput, View } from 'react-native';
import { ApiError, api } from '../../src/api';
import { RarityBadge, st } from '../../src/components/game';
import { Bar, Banner, Button, Card, Empty, Row, Screen, SectionLabel } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, radius, rarityColor, slotLabel, space, statBlurb, statLabel, text as t } from '../../src/theme';
import type { InventoryItem } from '../../src/types';

export default function DriverScreen() {
  const { state, act, refresh } = useGame();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [renaming, setRenaming] = useState(false);

  if (!state) return null;
  const p = state.player;

  const run = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Action failed');
    } finally {
      setBusy(null);
    }
  };

  const equippedBySlot = new Map(state.equipped.map((i) => [i.slot, i]));
  const spare = state.inventory.filter((i) => !i.equipped);

  return (
    <Screen onRefresh={refresh}>
      {error && <Banner tone="error">{error}</Banner>}

      <Card>
        <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <View style={{ flex: 1 }}>
            <Text style={t.label}>Driver</Text>
            {renaming ? (
              <NameEditor
                current={p.display_name}
                busy={busy === 'name'}
                onCancel={() => setRenaming(false)}
                onSave={(name) =>
                  run('name', async () => {
                    await api.post('/v1/me/name', { name });
                    await refresh();
                    setRenaming(false);
                  })
                }
              />
            ) : (
              <Row gap={2}>
                <Text style={t.title}>{p.display_name}</Text>
                <Text
                  style={[t.small, { color: colors.accent }]}
                  onPress={() => setRenaming(true)}>
                  edit
                </Text>
              </Row>
            )}
          </View>
          <View style={{ alignItems: 'flex-end' }}>
            <Text style={t.label}>Level</Text>
            <Text style={st.ratingValue}>{p.driver_level}</Text>
          </View>
        </Row>
        <View style={{ gap: space(1) }}>
          <Row style={{ justifyContent: 'space-between' }}>
            <Text style={t.small}>Experience</Text>
            <Text style={t.tiny}>
              {p.driver_xp.toLocaleString()} / {p.driver_xp_to_next.toLocaleString()}
            </Text>
          </Row>
          <Bar value={p.driver_xp} max={p.driver_xp_to_next} color={colors.blue} />
        </View>
      </Card>

      <SectionLabel
        right={
          p.unspent_points > 0 ? (
            <Text style={[t.small, { color: colors.accent }]}>{p.unspent_points} to spend</Text>
          ) : undefined
        }>
        Attributes
      </SectionLabel>

      {p.unspent_points === 0 && (
        <Text style={t.tiny}>
          Values below include equipment. Points are earned by levelling up, and are spent server-side.
        </Text>
      )}

      {(Object.keys(statLabel) as (keyof typeof statLabel)[]).map((key) => {
        const base = p.base_stats[key as keyof typeof p.base_stats];
        const total = state.build.driver[key as keyof typeof state.build.driver];
        const bonus = total - base;
        return (
          <Card key={key}>
            <Row style={{ justifyContent: 'space-between' }}>
              <Text style={t.heading}>{statLabel[key]}</Text>
              <Row gap={1}>
                <Text style={st.statValue}>{total.toFixed(0)}</Text>
                {bonus > 0.05 ? (
                  <Text style={[t.tiny, { color: colors.green }]}>(+{bonus.toFixed(1)})</Text>
                ) : null}
              </Row>
            </Row>
            <Text style={t.small}>{statBlurb[key]}</Text>
            {p.unspent_points > 0 && (
              <Button
                title="Spend a point"
                busy={busy === key}
                onPress={() => run(key, () => act('/v1/driver/allocate', { stat: key, points: 1 }))}
              />
            )}
          </Card>
        );
      })}

      <SectionLabel>Equipped</SectionLabel>
      <Card>
        {Object.keys(slotLabel).map((slot) => {
          const item = equippedBySlot.get(slot);
          return (
            <Row key={slot} style={{ justifyContent: 'space-between', paddingVertical: space(1) }}>
              <Text style={t.label}>{slotLabel[slot]}</Text>
              {item ? (
                <Row gap={2}>
                  <Text style={[t.small, { color: rarityColor[item.rarity] }]}>{item.name}</Text>
                  <Button
                    title="Remove"
                    variant="ghost"
                    busy={busy === `un${item.id}`}
                    style={{ paddingVertical: 4, paddingHorizontal: space(2), minHeight: 0 }}
                    onPress={() => run(`un${item.id}`, () => act('/v1/inventory/unequip', { item_id: item.id }))}
                  />
                </Row>
              ) : (
                <Text style={t.tiny}>empty</Text>
              )}
            </Row>
          );
        })}
      </Card>

      <SectionLabel>Inventory</SectionLabel>
      {spare.length === 0 ? (
        <Empty>Nothing spare. Items drop from jobs and from beating strong rivals.</Empty>
      ) : (
        spare.map((item) => (
          <ItemCard
            key={item.id}
            item={item}
            busy={busy}
            onEquip={() => run(`eq${item.id}`, () => act('/v1/inventory/equip', { item_id: item.id }))}
            onSell={() => run(`sell${item.id}`, () => act('/v1/inventory/sell', { item_id: item.id }))}
          />
        ))
      )}
    </Screen>
  );
}

function ItemCard({
  item,
  busy,
  onEquip,
  onSell,
}: {
  item: InventoryItem;
  busy: string | null;
  onEquip: () => void;
  onSell: () => void;
}) {
  return (
    <Card>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1, paddingRight: space(2) }}>
          <Text style={[t.heading, { color: rarityColor[item.rarity] }]}>{item.name}</Text>
          <Text style={t.tiny}>
            {slotLabel[item.slot] ?? item.slot} · quality {item.quality.toFixed(2)}
          </Text>
        </View>
        <RarityBadge rarity={item.rarity} />
      </Row>
      <Text style={[t.small, { fontStyle: 'italic' }]}>{item.flavour}</Text>
      <Row gap={2}>
        <Button title="Equip" variant="primary" style={{ flex: 1 }} busy={busy === `eq${item.id}`} onPress={onEquip} />
        <Button
          title={`Sell ¥${item.sell_value.toLocaleString()}`}
          variant="ghost"
          style={{ flex: 1 }}
          busy={busy === `sell${item.id}`}
          onPress={onSell}
        />
      </Row>
    </Card>
  );
}

/** Handle editing. The server enforces the 3–20 character rule; this only
 *  keeps the obviously-invalid cases from becoming a round trip. */
function NameEditor({
  current,
  busy,
  onSave,
  onCancel,
}: {
  current: string;
  busy: boolean;
  onSave: (name: string) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState(current);
  const trimmed = draft.trim();
  const valid = trimmed.length >= 3 && trimmed.length <= 20;

  return (
    <View style={{ gap: space(2), marginTop: space(1) }}>
      <TextInput
        value={draft}
        onChangeText={setDraft}
        autoFocus
        maxLength={20}
        selectTextOnFocus
        placeholder="Your handle on the mountain"
        placeholderTextColor={colors.textFaint}
        style={{
          color: colors.text,
          fontSize: 18,
          fontWeight: '700',
          backgroundColor: colors.surfaceAlt,
          borderWidth: 1,
          borderColor: valid ? colors.borderBright : colors.red,
          borderRadius: radius.md,
          paddingHorizontal: space(3),
          paddingVertical: space(2),
        }}
      />
      <Row gap={2}>
        <Button
          title="Save"
          variant="primary"
          style={{ flex: 1 }}
          disabled={!valid || trimmed === current}
          busy={busy}
          onPress={() => onSave(trimmed)}
        />
        <Button title="Cancel" variant="ghost" style={{ flex: 1 }} onPress={onCancel} />
      </Row>
      {!valid && <Text style={t.tiny}>3 to 20 characters.</Text>}
    </View>
  );
}
