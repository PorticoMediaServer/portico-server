import test from 'node:test';
import assert from 'node:assert/strict';
import {countLiterals} from './lint-strings.mjs';

const texts = (source, options) => countLiterals(source, options).hits.map(hit => hit.text);

test('TS generics, intersections and comparisons never become JSX text', () => {
  const source = `
    const scheduler = useMemo(() => new ArtworkScheduler<ArtworkImage>({load}), []);
    const owner = useMemo(() => new PlaybackMediaOwner<HTMLVideoElement>(), []);
    type Skeleton = Readonly<Props> & Pick<Layout, 'size'>;
    const accept = <Value extends object,>(value: Value): Value => value;
    const valid = count > 0 || preview?.blocked.length || acrossLibraries ? ready : blocked;
    const later = count < limit && count > first;
    const view = <Dialog>{count > 0 || preview?.blocked.length || acrossLibraries ? allowed : denied}</Dialog>;
  `;
  assert.deepEqual(texts(source), []);
});

test('actual JSX text still counts code-looking words and punctuation', () => {
  assert.deepEqual(texts(`const view = <><p>new ArtworkScheduler</p><p>new PlaybackMediaOwner</p><p>Pick a library</p><p>Ready (2); continue?</p><p>Save &amp; continue</p></>;`),
    ['new ArtworkScheduler', 'new PlaybackMediaOwner', 'Pick a library', 'Ready (2); continue?', 'Save &amp; continue']); // Diagnostics retain JSX source spelling.
});

test('copy props, accessibility names, object values and assignment positions remain checked once', () => {
  const source = `
    const view = <Button title="Open details" aria-label={'Play movie'} accessibilityHint={"Starts playback"} label={ready ? 'Play now' : 'Try again'} />;
    const options = {title: 'Actual title', detail: '4:10 PM', action: ready ? 'Save changes' : 'Try again'};
    const message = 'Something happened';
    target.caption = 'Episode one';
    class Panel { help = 'Choose a library'; }
  `;
  assert.deepEqual(texts(source), ['Open details', 'Play movie', 'Starts playback', 'Play now', 'Try again', 'Actual title', '4:10 PM', 'Save changes', 'Try again', 'Something happened', 'Episode one', 'Choose a library']);
});

test('nested JSX expressions, multiline state objects and template substitutions stay visible', () => {
  const source = [
    'const view = <Panel',
    '  title={ready ? `Ready for ${name}` : "Try again"}',
    '  body={ready ? {message: "Saved successfully"} : {message: "Could not save"}}',
    '>',
    '  Choose a library',
    '  {ready ? <b>Play now</b> : render({label: "Try again"})}',
    '  {`Playing ${ready ? "Movie title" : "Another title"}`}',
    '</Panel>;',
  ].join('\n');
  const result = countLiterals(source);
  assert.deepEqual(result.hits, [
    {line: 2, text: 'Ready for ${x}'}, {line: 2, text: 'Try again'},
    {line: 3, text: 'Saved successfully'}, {line: 3, text: 'Could not save'},
    {line: 5, text: 'Choose a library'}, {line: 6, text: 'Play now'}, {line: 6, text: 'Try again'},
    {line: 7, text: 'Playing ${x}'}, {line: 7, text: 'Movie title'}, {line: 7, text: 'Another title'},
  ]);
});

test('escaped quotes and operator characters inside real copy are not code exclusions', () => {
  const source = String.raw`const view = <Panel title={'Owner\'s library'} body={"Say \"Play now\""}><p>{ready ? 'Try again && continue' : 'Ready === yes'}</p></Panel>;`;
  assert.deepEqual(texts(source), ["Owner's library", 'Say "Play now"', 'Try again && continue', 'Ready === yes']);
});

test('comments, type literals, identifiers and catalog keys remain inert', () => {
  const source = `
    // title: 'Comment only'
    /* <p>Comment only</p> */
    type View = {title: 'Type only'; label: string};
    const source = "<p>String containing markup</p>";
    const view = <Panel title={t('next.dock.nowPlaying')} aria-label={identity.title} />;
    const options = {title: 'saved.tab.favorites', help: '/v1/items'};
  `;
  assert.deepEqual(texts(source), []);
  assert.equal(countLiterals("const options = {title: 'Actual title'};", {jsx: false}).count, 1);
  assert.deepEqual(texts("namespace Screen { export const View = () => <p>Actual copy</p>; }"), ['Actual copy']);
});

test('the existing independent snippet contract keeps its substantive count', () => {
  const source = [
    '<Button variant="ghost" label="Try again" onClick={retry} />',
    '<Text>Nothing here yet</Text>',
    "{id: 'a', label: 'Add to Watchlist'}",
    "<Button label={i18n.t('action.refresh')} />",
    '<Text>{count > 1 ? a : b}</Text>',
    '<Text>Deliberate</Text> // lint-strings-allow',
  ].join('\n');
  assert.equal(countLiterals(source).count, 3);
});

test('malformed multiline source fails closed instead of losing checked positions', () => {
  assert.throws(() => countLiterals('const view = <Panel\n title="Actual title"'), /cannot parse source/);
});
