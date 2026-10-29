import test from 'node:test';
import assert from 'node:assert/strict';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {defaultI18n} from '@i18n';
import {LibraryEmpty} from '../src/kit/states/LibraryEmpty.tsx';

const props = {api: {} as never, scope: {serverId: 's', viewerId: 'v'} as never, libraryId: 'books', libraryName: 'Books', onActivity: () => {}};

test('P2-59: a Reading image still being built is the calm first-scan state for anyone, never an exact empty', () => {
  const member = renderToStaticMarkup(createElement(LibraryEmpty, {...props, owner: false, importing: true}));
  assert.ok(member.includes('Titles appear here as they'), 'the first-scan words');
  assert.ok(!member.includes(defaultI18n.t('next.states.viewActivity')), 'a member is offered no activity');
  assert.ok(!member.includes(defaultI18n.t('library.emptyLibrary')), 'never the settled empty-library words');
  const settled = renderToStaticMarkup(createElement(LibraryEmpty, {...props, owner: false}));
  assert.ok(settled.includes(defaultI18n.t('library.emptyLibrary')));
  const owner = renderToStaticMarkup(createElement(LibraryEmpty, {...props, owner: true, importing: true}));
  assert.ok(owner.includes(defaultI18n.t('next.states.viewActivity')), 'the owner can open activity');
});
