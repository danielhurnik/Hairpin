import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  PanResponder,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
  type LayoutChangeEvent,
} from 'react-native';
import { colors, font, radius, space, text as t } from '../theme';

/**
 * One adjustment, set the way it would be set on the car.
 *
 * Three ways in, because there are three things a player wants to do with a
 * setting and no single control does all of them: drag the bar to sweep the
 * range and feel where the ends are, tap the arrows to walk a step at a time,
 * or type the number when you already know it is 87mm you want.
 *
 * Everything here is in the axis's own units. The normalised position the
 * model works in never appears on screen.
 */
export function Dial({
  label,
  sublabel,
  value,
  low,
  high,
  step,
  decimals,
  unit,
  lowLabel,
  highLabel,
  factory,
  busy,
  disabled,
  onCommit,
}: {
  label: string;
  sublabel?: string;
  /** The current setting, in real units. */
  value: number;
  /** The ends of the range. `low` may be the larger number — winding a car down is fewer millimetres. */
  low: number;
  high: number;
  step: number;
  decimals: number;
  unit: string;
  lowLabel: string;
  highLabel: string;
  factory: number;
  busy?: boolean;
  disabled?: boolean;
  onCommit: (setting: number) => void;
}) {
  const [width, setWidth] = useState(0);
  // What the control is showing right now. It runs ahead of the server while a
  // drag is in flight so the bar tracks the finger, and falls back into line
  // with whatever the server says the moment the write lands.
  const [live, setLive] = useState(value);
  const [typing, setTyping] = useState<string | null>(null);
  const dragging = useRef(false);

  useEffect(() => {
    if (!dragging.current) setLive(value);
  }, [value]);

  const span = high - low;
  const fraction = span === 0 ? 0 : clamp01((live - low) / span);
  const factoryAt = span === 0 ? 0 : clamp01((factory - low) / span);

  const snap = (raw: number) => {
    const stepped = factory + Math.round((raw - factory) / step) * step;
    const lo = Math.min(low, high);
    const hi = Math.max(low, high);
    return Math.min(hi, Math.max(lo, stepped));
  };

  const fromX = (x: number) => snap(low + (span * clamp01(width === 0 ? 0 : x / width)));

  const pan = useMemo(
    () =>
      PanResponder.create({
        onStartShouldSetPanResponder: () => !disabled,
        onMoveShouldSetPanResponder: () => !disabled,
        onPanResponderGrant: (e) => {
          dragging.current = true;
          setLive(fromX(e.nativeEvent.locationX));
        },
        onPanResponderMove: (e) => setLive(fromX(e.nativeEvent.locationX)),
        onPanResponderRelease: (e) => {
          const settled = fromX(e.nativeEvent.locationX);
          dragging.current = false;
          setLive(settled);
          if (settled !== value) onCommit(settled);
        },
        onPanResponderTerminate: () => {
          dragging.current = false;
          setLive(value);
        },
      }),
    // fromX closes over width and the range; rebuild when those move.
    [disabled, width, low, high, step, factory, value, onCommit]
  );

  const nudge = (direction: number) => {
    const next = snap(live + direction * step);
    if (next === live) return;
    setLive(next);
    onCommit(next);
  };

  const commitTyped = () => {
    const raw = typing;
    setTyping(null);
    if (raw === null) return;
    const parsed = Number(raw.replace(',', '.'));
    if (!Number.isFinite(parsed)) return;
    const next = snap(parsed);
    setLive(next);
    if (next !== value) onCommit(next);
  };

  const shown = typing ?? live.toFixed(decimals);

  return (
    <View style={[s.wrap, disabled && s.off]}>
      <View style={s.head}>
        <View style={{ flex: 1 }}>
          <Text style={t.heading}>{label}</Text>
          {sublabel ? <Text style={t.tiny}>{sublabel}</Text> : null}
        </View>
        <View style={s.entry}>
          <TextInput
            value={shown}
            editable={!disabled}
            onChangeText={setTyping}
            onBlur={commitTyped}
            onSubmitEditing={commitTyped}
            keyboardType="numbers-and-punctuation"
            returnKeyType="done"
            selectTextOnFocus
            style={s.input}
            accessibilityLabel={`${label} setting`}
          />
          {unit ? <Text style={s.unit}>{unit}</Text> : null}
        </View>
      </View>

      <View style={s.controls}>
        <Stepper label="−" onPress={() => nudge(-1)} disabled={disabled || busy} />
        <View style={s.barArea} onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)}>
          <View style={s.hit} {...pan.panHandlers}>
            <View style={s.barTrack}>
              <View style={[s.barFill, { width: `${fraction * 100}%` }]} />
              <View style={[s.factoryTick, { left: `${factoryAt * 100}%` }]} />
              <View style={[s.thumb, { left: `${fraction * 100}%` }]} />
            </View>
          </View>
        </View>
        <Stepper label="+" onPress={() => nudge(1)} disabled={disabled || busy} />
      </View>

      <View style={s.ends}>
        <Text style={t.tiny}>
          {lowLabel} · {low.toFixed(decimals)}
        </Text>
        <Text style={t.tiny}>
          {high.toFixed(decimals)} · {highLabel}
        </Text>
      </View>
    </View>
  );
}

function Stepper({
  label,
  onPress,
  disabled,
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  return (
    <Pressable
      onPress={disabled ? undefined : onPress}
      disabled={disabled}
      hitSlop={6}
      style={({ pressed }) => [s.step, pressed && !disabled && { opacity: 0.6 }]}>
      <Text style={[s.stepText, disabled && { color: colors.textFaint }]}>{label}</Text>
    </Pressable>
  );
}

const clamp01 = (n: number) => (n < 0 ? 0 : n > 1 ? 1 : n);

const s = StyleSheet.create({
  wrap: { gap: space(2) },
  off: { opacity: 0.45 },
  head: { flexDirection: 'row', alignItems: 'flex-start', gap: space(2) },
  // Fixed width on the field and the unit, so the name of the axis gets
  // whatever is left rather than being squeezed into one letter per line.
  entry: { flexDirection: 'row', alignItems: 'baseline', gap: space(1), flexShrink: 0 },
  input: {
    color: colors.text,
    fontFamily: font.mono,
    fontSize: 19,
    fontWeight: '700',
    textAlign: 'right',
    width: 84,
    paddingVertical: 3,
    paddingHorizontal: space(2),
    borderRadius: radius.sm,
    backgroundColor: colors.raised,
  },
  unit: { color: colors.textDim, fontSize: 11, width: 42 },
  controls: { flexDirection: 'row', alignItems: 'center', gap: space(2) },
  barArea: { flex: 1 },
  // A tall invisible strip so the bar can be dragged without hunting for 8px.
  hit: { paddingVertical: space(3), justifyContent: 'center' },
  barTrack: {
    height: 8,
    borderRadius: 4,
    backgroundColor: colors.raised,
    justifyContent: 'center',
  },
  barFill: { height: 8, borderRadius: 4, backgroundColor: colors.accentDim },
  factoryTick: {
    position: 'absolute',
    width: 2,
    height: 14,
    marginLeft: -1,
    backgroundColor: colors.textFaint,
  },
  thumb: {
    position: 'absolute',
    width: 16,
    height: 16,
    marginLeft: -8,
    borderRadius: 8,
    backgroundColor: colors.accent,
    borderWidth: 2,
    borderColor: colors.bg,
  },
  step: {
    width: 34,
    height: 34,
    borderRadius: radius.sm,
    borderWidth: 1,
    borderColor: colors.borderBright,
    alignItems: 'center',
    justifyContent: 'center',
  },
  stepText: { color: colors.text, fontSize: 18, fontWeight: '700', lineHeight: 20 },
  ends: { flexDirection: 'row', justifyContent: 'space-between' },
});
