import { Tabs } from 'expo-router';
import React from 'react';
import { Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Money } from '../../src/components/game';
import { PixelIcon } from '../../src/pixel/sprite';
import { useGame } from '../../src/state';
import { colors, space, text as t } from '../../src/theme';

function TabIcon({ name, focused }: { name: string; focused: boolean }) {
  return <PixelIcon name={name} color={focused ? colors.accent : colors.textFaint} size={22} />;
}

/** Always-visible currency and rank, so the numbers on screen are the server's. */
function TopBar() {
  const { state } = useGame();
  const insets = useSafeAreaInsets();
  if (!state) return null;
  const p = state.player;
  return (
    <View
      style={{
        paddingTop: insets.top + space(2),
        paddingBottom: space(2),
        paddingHorizontal: space(4),
        backgroundColor: colors.surface,
        borderBottomWidth: 1,
        borderBottomColor: colors.border,
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
      }}>
      <View>
        <Text style={[t.heading, { fontSize: 14 }]}>{p.display_name}</Text>
        <Text style={t.tiny}>
          Rank {p.ladder_rank} · Level {p.driver_level} · {p.wins}W {p.losses}L
        </Text>
      </View>
      <Money cash={p.cash} rubies={p.rubies} />
    </View>
  );
}

export default function TabsLayout() {
  return (
    <>
      <TopBar />
      <Tabs
        screenOptions={{
          headerShown: false,
          tabBarStyle: {
            backgroundColor: colors.surface,
            borderTopColor: colors.border,
          },
          tabBarActiveTintColor: colors.accent,
          tabBarInactiveTintColor: colors.textFaint,
          tabBarLabelStyle: { fontSize: 10, fontWeight: '700' },
          sceneStyle: { backgroundColor: colors.bg },
        }}>
        <Tabs.Screen
          name="index"
          options={{
            title: 'Garage',
            tabBarIcon: ({ focused }) => <TabIcon name="garage" focused={focused} />,
          }}
        />
        <Tabs.Screen
          name="jobs"
          options={{
            title: 'Jobs',
            tabBarIcon: ({ focused }) => <TabIcon name="jobs" focused={focused} />,
          }}
        />
        <Tabs.Screen
          name="race"
          options={{
            title: 'Race',
            tabBarIcon: ({ focused }) => <TabIcon name="race" focused={focused} />,
          }}
        />
        <Tabs.Screen
          name="ladder"
          options={{
            title: 'Ladder',
            tabBarIcon: ({ focused }) => <TabIcon name="ladder" focused={focused} />,
          }}
        />
        <Tabs.Screen
          name="driver"
          options={{
            title: 'Driver',
            tabBarIcon: ({ focused }) => <TabIcon name="driver" focused={focused} />,
          }}
        />
        <Tabs.Screen
          name="shop"
          options={{
            title: "K's Office",
            tabBarIcon: ({ focused }) => <TabIcon name="shop" focused={focused} />,
          }}
        />
      </Tabs>
    </>
  );
}
