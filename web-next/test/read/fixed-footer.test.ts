import assert from 'node:assert/strict';
import {test} from 'node:test';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {ReaderChrome, type ReaderChromeProps} from '../../src/kit/reader/ReaderChrome.tsx';
import type {ReaderChromePresentation} from '../../src/screens/read/ReadReader.tsx';

const fixedPageFooter: ReaderChromePresentation['fixedPageFooter'] = createElement('span', {'data-test-fixed-footer': true}, 'Observed fit');
const props: ReaderChromeProps = {
  copy: {back: 'Back', previous: 'Previous', next: 'Next', progress: 'Position'},
  direction: 'ltr', tapTurnEnabled: true, pageTurnsEnabled: true, showPageArrows: false,
  onBack() {}, onTurn() {}, actions: [], children: null, pages: 'Observed page', fixedPageFooter,
};
const render = (overrides: Partial<ReaderChromeProps>) => renderToStaticMarkup(createElement(ReaderChrome, {...props, ...overrides}));

test('fixed-page footer uses the supplied assembly slot inside the existing hidden chrome', () => {
  const html = render({fixedPage: true, initiallyVisible: false});
  assert.match(html, /data-test-fixed-footer/);
  assert.doesNotMatch(html, /Observed page/);
  assert.match(html, /aria-hidden="true"/);
  assert.match(html, /inert=""/);
});

test('prose ignores a fixed-page slot and fixed pages without one retain the generic footer', () => {
  for (const overrides of [{fixedPage: false}, {fixedPage: true, fixedPageFooter: undefined}, {fixedPage: true, fixedPageFooter: null}]) {
    const html = render(overrides);
    assert.match(html, /Observed page/);
    assert.doesNotMatch(html, /data-test-fixed-footer/);
  }
});
