export type LyricCue = {
  id: string;
  startSeconds: number;
  text: string;
};

export type LyricMetadata = {
  album?: string;
  artist?: string;
  author?: string;
  language?: string;
  lengthSeconds?: number;
  offsetMilliseconds: number;
  title?: string;
};

export type ParsedLyrics = {
  cues: LyricCue[];
  metadata: LyricMetadata;
  plainText: string;
};

export type LyricSource = {
  id: string;
  synced: boolean;
  text?: string;
};

export type LyricDocument = ParsedLyrics & {
  sourceId: string;
  synchronized: boolean;
};

const TIMESTAMP_SOURCE = String.raw`\[(\d{1,3}):([0-5]?\d)(?:[.:](\d{1,3}))?\]`;
const WORD_TIMESTAMP_SOURCE = String.raw`<(?:\d{1,3}):(?:[0-5]?\d)(?:[.:]\d{1,3})?>`;
const BROAD_TIMESTAMP_SOURCE = String.raw`\[(?:\d{1,3}:){1,2}\d{1,2}(?:[.:]\d{1,3})?\]`;
const METADATA_TAG_SOURCE = String.raw`\[(ar|al|ti|au|by|la|length|offset|re|ve):([^\]]*)\]`;
const MAX_OFFSET_MILLISECONDS = 10 * 60 * 1_000;
const MAX_METADATA_LENGTH = 256;

function timestampPattern() {
  return new RegExp(TIMESTAMP_SOURCE, 'g');
}

function wordTimestampPattern() {
  return new RegExp(WORD_TIMESTAMP_SOURCE, 'g');
}

function broadTimestampPattern() {
  return new RegExp(BROAD_TIMESTAMP_SOURCE, 'g');
}

function metadataPattern() {
  return new RegExp(METADATA_TAG_SOURCE, 'gi');
}

function normalizeInput(value: string) {
  return value.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n');
}

function safeMetadataValue(value: string) {
  return value.replace(/[\u0000-\u001F\u007F]/g, ' ').replace(/\s+/g, ' ').trim().slice(0, MAX_METADATA_LENGTH);
}

function timestampFromMatch(match: RegExpMatchArray) {
  const minutes = Number(match[1]);
  const seconds = Number(match[2]);
  const fraction = match[3] ? Number(`0.${match[3]}`) : 0;
  if (![minutes, seconds, fraction].every(Number.isFinite)) return undefined;
  if (seconds >= 60) return undefined;
  return minutes * 60 + seconds + fraction;
}

function parseLength(value: string) {
  const match = `[${safeMetadataValue(value)}]`.match(new RegExp(`^${TIMESTAMP_SOURCE}$`));
  return match ? timestampFromMatch(match) : undefined;
}

function metadataFor(input: string): LyricMetadata {
  const metadata: LyricMetadata = { offsetMilliseconds: 0 };
  for (const match of input.matchAll(metadataPattern())) {
    const tag = match[1].toLowerCase();
    const value = safeMetadataValue(match[2]);
    if (!value) continue;
    if (tag === 'offset') {
      const offset = /^[+-]?\d+$/.test(value) ? Number.parseInt(value, 10) : Number.NaN;
      if (Number.isFinite(offset)) metadata.offsetMilliseconds = Math.max(-MAX_OFFSET_MILLISECONDS, Math.min(MAX_OFFSET_MILLISECONDS, offset));
    } else if (tag === 'length') {
      const lengthSeconds = parseLength(value);
      if (lengthSeconds !== undefined) metadata.lengthSeconds = lengthSeconds;
    } else if (tag === 'ar' && !metadata.artist) metadata.artist = value;
    else if (tag === 'al' && !metadata.album) metadata.album = value;
    else if (tag === 'ti' && !metadata.title) metadata.title = value;
    else if ((tag === 'au' || tag === 'by') && !metadata.author) metadata.author = value;
    else if (tag === 'la' && !metadata.language) metadata.language = value;
  }
  return metadata;
}

function lyricTextForLine(line: string) {
  return line
    .replace(timestampPattern(), '')
    .replace(wordTimestampPattern(), '')
    .replace(metadataPattern(), '')
    .trim();
}

function normalizePlainLines(lines: string[]) {
  const normalized: string[] = [];
  let previousBlank = true;
  for (const line of lines) {
    const text = line
      .replace(timestampPattern(), '')
      .replace(broadTimestampPattern(), '')
      .replace(wordTimestampPattern(), '')
      .replace(metadataPattern(), '')
      .trimEnd();
    const blank = !text.trim();
    if (blank && previousBlank) continue;
    normalized.push(blank ? '' : text.trimStart());
    previousBlank = blank;
  }
  while (normalized.at(-1) === '') normalized.pop();
  return normalized.join('\n');
}

export function parseLyrics(raw: string): ParsedLyrics {
  const input = normalizeInput(raw);
  const metadata = metadataFor(input);
  const offsetSeconds = metadata.offsetMilliseconds / 1_000;
  const cueGroups = new Map<number, { order: number; texts: string[] }>();
  let order = 0;

  for (const line of input.split('\n')) {
    const matches = [...line.matchAll(timestampPattern())];
    if (!matches.length) continue;
    const text = lyricTextForLine(line);
    for (const match of matches) {
      const parsedTimestamp = timestampFromMatch(match);
      if (parsedTimestamp === undefined) continue;
      const startMilliseconds = Math.max(0, Math.round((parsedTimestamp + offsetSeconds) * 1_000));
      const group = cueGroups.get(startMilliseconds) ?? { order: order++, texts: [] };
      if (!group.texts.includes(text)) group.texts.push(text);
      cueGroups.set(startMilliseconds, group);
    }
  }

  const cues = [...cueGroups.entries()]
    .sort(([firstTime, first], [secondTime, second]) => firstTime - secondTime || first.order - second.order)
    .map(([startMilliseconds, group], index) => ({
      id: `${startMilliseconds}-${index}`,
      startSeconds: startMilliseconds / 1_000,
      text: group.texts.filter(Boolean).join('\n'),
    }));

  return {
    cues,
    metadata,
    plainText: normalizePlainLines(input.split('\n')),
  };
}

export function selectLyricDocument(sources: LyricSource[]): LyricDocument | undefined {
  const parsed = sources
    .filter((source) => Boolean(source.text?.trim()))
    .map((source) => ({ source, ...parseLyrics(source.text ?? '') }));
  const synchronized = parsed.find(({ source, cues }) => source.synced && cues.length >= 2 && cues.some((cue) => cue.text));
  const selected = synchronized ?? parsed.find(({ plainText }) => Boolean(plainText));
  if (!selected) return undefined;
  return {
    sourceId: selected.source.id,
    synchronized: selected === synchronized,
    cues: selected.cues,
    metadata: selected.metadata,
    plainText: selected.plainText,
  };
}

export function activeLyricCueIndex(cues: LyricCue[], currentTimeSeconds: number) {
  if (!Number.isFinite(currentTimeSeconds) || currentTimeSeconds < 0 || !cues.length) return -1;
  let low = 0;
  let high = cues.length - 1;
  let active = -1;
  while (low <= high) {
    const middle = Math.floor((low + high) / 2);
    if (cues[middle].startSeconds <= currentTimeSeconds) {
      active = middle;
      low = middle + 1;
    } else {
      high = middle - 1;
    }
  }
  return active;
}
