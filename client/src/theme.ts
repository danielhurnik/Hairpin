// Dark, low-glare palette — the game is set at 2am on a mountain.

export const colors = {
  bg: '#0B0C0F',
  surface: '#14161B',
  surfaceAlt: '#1B1E25',
  raised: '#232730',
  border: '#282D37',
  borderBright: '#3A4150',

  text: '#E9ECF3',
  textDim: '#9AA3B4',
  textFaint: '#5D6675',

  accent: '#F2B33D', // headlight amber
  accentDim: '#8A6A26',
  red: '#E0344B',
  green: '#3CCB7F',
  blue: '#4E8BF0',
  purple: '#A06BE0',

  cash: '#3CCB7F',
  ruby: '#E0446B',
} as const;

export const rarityColor: Record<string, string> = {
  common: '#8A93A5',
  uncommon: '#4FB477',
  rare: '#4E8BF0',
  epic: '#A06BE0',
  legendary: '#F2B33D',
};

export const rarityLabel: Record<string, string> = {
  common: 'Common',
  uncommon: 'Uncommon',
  rare: 'Rare',
  epic: 'Epic',
  legendary: 'Legendary',
};

export const slotLabel: Record<string, string> = {
  head: 'Head',
  hands: 'Hands',
  feet: 'Feet',
  suit: 'Suit',
  mind: 'Mind',
  charm: 'Charm',
  car: 'Car Part',
};

export const statLabel: Record<string, string> = {
  bravery: 'Bravery',
  intelligence: 'Intelligence',
  reflexes: 'Reflexes',
  consistency: 'Consistency',
  stamina: 'Stamina',
};

export const statBlurb: Record<string, string> = {
  bravery: 'Commitment through fast corners and late braking — but it invites mistakes.',
  intelligence: 'Reading the road ahead. Better lines, fewer errors.',
  reflexes: 'Catching the car. Wins hairpins and quick direction changes.',
  consistency: 'Repeatability. Narrows your spread and steadies your hands.',
  stamina: 'Concentration late in a run, when the last corners decide it.',
};

export const space = (n: number) => n * 4;

export const radius = { sm: 6, md: 10, lg: 14, pill: 999 } as const;

export const font = {
  mono: 'monospace',
} as const;

export const text = {
  title: { fontSize: 22, fontWeight: '700' as const, color: colors.text, letterSpacing: 0.3 },
  heading: { fontSize: 16, fontWeight: '700' as const, color: colors.text, letterSpacing: 0.4 },
  body: { fontSize: 14, color: colors.text },
  small: { fontSize: 12, color: colors.textDim },
  tiny: { fontSize: 11, color: colors.textFaint },
  label: {
    fontSize: 10,
    fontWeight: '700' as const,
    color: colors.textFaint,
    letterSpacing: 1.2,
    textTransform: 'uppercase' as const,
  },
};
